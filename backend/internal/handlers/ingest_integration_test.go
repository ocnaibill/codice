package handlers

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/redis/go-redis/v9"
)

func multipartBody(field, filename string, content []byte) (*bytes.Buffer, string) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if filename != "" {
		fw, _ := mw.CreateFormFile(field, filename)
		fw.Write(content)
	} else {
		mw.WriteField(field, "no file here")
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

func (s *catalogStack) upload(a actor, filename string, content []byte) *httptest.ResponseRecorder {
	s.t.Helper()
	body, ct := multipartBody("document", filename, content)
	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-Test-User", a.id)
	req.Header.Set("X-Test-Role", a.role)
	rec := httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	return rec
}

// leftovers lists what remains in the storage directory: stored files and any
// staging debris.
func (s *catalogStack) stored() []string {
	var out []string
	filepath.Walk(s.storage, func(p string, info os.FileInfo, err error) error {
		if err == nil && info.Mode().IsRegular() {
			rel, _ := filepath.Rel(s.storage, p)
			out = append(out, rel)
		}
		return nil
	})
	return out
}

func (s *catalogStack) counts() string {
	return s.scalar(`SELECT (SELECT count(*) FROM works) || '/' || (SELECT count(*) FROM files) || '/' || (SELECT count(*) FROM jobs)`)
}

func TestUpload_StoresHashesAndQueuesInOneStep(t *testing.T) {
	s := newCatalogStack(t)
	content := corpusFile(t, "cbz_ltr.cbz")

	rec := s.upload(admin, "Watchmen 01.cbz", content)
	if rec.Code != 200 {
		t.Fatalf("upload: %d %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		WorkID int   `json:"work_id"`
		JobID  int64 `json:"job_id"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)

	sum := s.scalar(`SELECT sha256 FROM files WHERE id = (SELECT file_id FROM work_primary WHERE work_id = $1)`, resp.WorkID)
	if len(sum) != 64 {
		t.Fatalf("hash not stored: %q", sum)
	}
	if got := s.scalar(`SELECT size_bytes FROM files WHERE sha256 = $1`, sum); got != fmt.Sprint(len(content)) {
		t.Errorf("size = %s, want %d", got, len(content))
	}
	if got := s.scalar(`SELECT file_format FROM work_primary WHERE work_id = $1`, resp.WorkID); got != "cbz" {
		t.Errorf("format = %q", got)
	}
	// The job exists, waits for a worker, at manual priority, on behalf of the uploader.
	if got := s.scalar(`SELECT state || '/' || priority || '/' || created_by::text FROM jobs WHERE id = $1`, resp.JobID); got != "pending/10/"+idAdmin {
		t.Errorf("job = %q", got)
	}
	if got := s.scalar(`SELECT media_status FROM works WHERE id = $1`, resp.WorkID); got != "QUEUED" {
		t.Errorf("media_status = %q", got)
	}
	path := s.scalar(`SELECT payload->>'file_path' FROM jobs WHERE id = $1`, resp.JobID)
	if b, err := os.ReadFile(path); err != nil || !bytes.Equal(b, content) {
		t.Errorf("the queued file is not the uploaded bytes: %v", err)
	}
	// One file in storage, no staging debris, and a name that is not just the original.
	files := s.stored()
	if len(files) != 1 || !strings.HasSuffix(files[0], "_Watchmen 01.cbz") || strings.Contains(files[0], ".staging") {
		t.Errorf("storage = %v", files)
	}
}

func TestUpload_IdenticalBytesReturnTheExistingRecord(t *testing.T) {
	s := newCatalogStack(t)
	content := corpusFile(t, "epub_acentos.epub")

	first := s.upload(admin, "duna.epub", content)
	var created struct {
		WorkID int `json:"work_id"`
	}
	json.Unmarshal(first.Body.Bytes(), &created)

	// The corpus duplicate has another name and the same bytes: the hash decides, not the name.
	rec := s.upload(admin, "epub_duplicata.epub", corpusFile(t, "epub_duplicata.epub"))
	if rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d, want 409", rec.Code)
	}
	var dup struct {
		Error   string `json:"error"`
		WorkID  int    `json:"work_id"`
		FileID  int64  `json:"file_id"`
		Title   string `json:"title"`
		Retired bool   `json:"retired"`
	}
	json.Unmarshal(rec.Body.Bytes(), &dup)
	if dup.Error != "duplicate" || dup.WorkID != created.WorkID || dup.Title != "duna.epub" || dup.FileID == 0 {
		t.Errorf("duplicate answer = %+v", dup)
	}
	if got := s.counts(); got != "1/1/1" {
		t.Errorf("works/files/jobs = %s, want 1/1/1 (nothing new may be stored)", got)
	}
	if files := s.stored(); len(files) != 1 {
		t.Errorf("a second copy was stored: %v", files)
	}

	// A retired work still owns its bytes, and the answer says so.
	s.do(admin, "DELETE", fmt.Sprintf("/works/%d", created.WorkID), "")
	rec = s.upload(admin, "again.epub", content)
	json.Unmarshal(rec.Body.Bytes(), &dup)
	if rec.Code != http.StatusConflict || !dup.Retired {
		t.Errorf("duplicate of a retired work: %d retired=%v", rec.Code, dup.Retired)
	}

	// A file that only differs by a few bytes is a different file.
	if rec := s.upload(admin, "alterado.epub", corpusFile(t, "epub_alterado.epub")); rec.Code != 200 {
		t.Errorf("a different file was refused: %d", rec.Code)
	}
}

func TestUpload_SimultaneousUploadsOfTheSameBytesStoreOne(t *testing.T) {
	s := newCatalogStack(t)
	content := corpusFile(t, "pdf_digital.pdf")

	var wg sync.WaitGroup
	codes := make([]int, 8)
	start := make(chan struct{})
	for i := range codes {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i] = s.upload(admin, fmt.Sprintf("copy-%d.pdf", i), content).Code
		}(i)
	}
	close(start)
	wg.Wait()

	created, conflicts := 0, 0
	for _, c := range codes {
		switch c {
		case 200:
			created++
		case 409:
			conflicts++
		}
	}
	if created != 1 || conflicts != 7 {
		t.Errorf("codes = %v, want one 200 and seven 409", codes)
	}
	if got := s.counts(); got != "1/1/1" {
		t.Errorf("works/files/jobs = %s, want 1/1/1", got)
	}
	if files := s.stored(); len(files) != 1 {
		t.Errorf("storage after the race = %v, want exactly one file and no staging debris", files)
	}
}

func TestUpload_SameNameAtTheSameTimeNeverCollides(t *testing.T) {
	s := newCatalogStack(t)
	// Different bytes, same file name, back to back (the old name used the clock in seconds).
	s.upload(admin, "livro.epub", corpusFile(t, "epub_acentos.epub"))
	s.upload(admin, "livro.epub", corpusFile(t, "epub_alterado.epub"))
	names := s.scalar(`SELECT count(DISTINCT path) FROM storage_locations`)
	if names != "2" || s.counts() != "2/2/2" {
		t.Errorf("distinct stored names = %s, counts = %s", names, s.counts())
	}
}

func TestUpload_RefusesBadContentSizeAndFormat(t *testing.T) {
	s := newCatalogStack(t)

	for name, tc := range map[string]struct {
		file    string
		content []byte
		want    int
	}{
		"a corrupted EPUB":    {"quebrado.epub", corpusFile(t, "epub_corrompido.epub"), 415},
		"text saved as PDF":   {"falso.pdf", corpusFile(t, "falso.pdf"), 415},
		"an empty file":       {"vazio.epub", []byte{}, 415},
		"an unsupported type": {"programa.exe", []byte("MZ"), 400},
		"an unsupported name": {"sem-extensao", []byte("x"), 400},
	} {
		if rec := s.upload(admin, tc.file, tc.content); rec.Code != tc.want {
			t.Errorf("%s: got %d (%s), want %d", name, rec.Code, strings.TrimSpace(rec.Body.String()), tc.want)
		}
	}
	if got := s.counts(); got != "0/0/0" {
		t.Errorf("a refused upload left records behind: %s", got)
	}
	if files := s.stored(); len(files) != 0 {
		t.Errorf("a refused upload left files behind: %v", files)
	}

	// The size limit is real: it applies to the bytes, not to memory.
	t.Setenv("CODICE_MAX_UPLOAD_MB", "1")
	big := bytes.Repeat([]byte("a"), 2<<20)
	if rec := s.upload(admin, "grande.txt", big); rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("oversize upload: %d, want 413", rec.Code)
	}
	if got := s.counts(); got != "0/0/0" || len(s.stored()) != 0 {
		t.Errorf("an oversize upload left something behind: %s %v", got, s.stored())
	}
	// Just under the limit is fine.
	if rec := s.upload(admin, "cabe.txt", bytes.Repeat([]byte("a"), 900<<10)); rec.Code != 200 {
		t.Errorf("an upload under the limit: %d", rec.Code)
	}

	// Malformed requests.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/upload", strings.NewReader("just text"))
	req.Header.Set("X-Test-User", idAdmin)
	req.Header.Set("X-Test-Role", "admin")
	s.router.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("not a multipart form: %d, want 400", rec.Code)
	}
	body, ct := multipartBody("document", "", nil)
	req = httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("X-Test-User", idAdmin)
	req.Header.Set("X-Test-Role", "admin")
	rec = httptest.NewRecorder()
	s.router.ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Errorf("form without a document: %d, want 400", rec.Code)
	}
}

func TestUpload_AFailureAfterTheFileIsStoredLeavesNothingBehind(t *testing.T) {
	s := newCatalogStack(t)
	// Make the job insert fail, as if the queue were broken.
	s.exec(`ALTER TABLE jobs ADD CONSTRAINT refuse_everything CHECK (type <> 'ingest')`)
	if rec := s.upload(admin, "duna.epub", corpusFile(t, "epub_acentos.epub")); rec.Code != 500 {
		t.Fatalf("got %d, want 500", rec.Code)
	}
	if got := s.counts(); got != "0/0/0" {
		t.Errorf("a failed enqueue left a work or job behind: %s (the work and the job are one transaction)", got)
	}
	if files := s.stored(); len(files) != 0 {
		t.Errorf("a failed enqueue left files behind: %v", files)
	}
}

func TestUpload_WorksWhenRedisIsDown(t *testing.T) {
	s := newCatalogStack(t)
	down := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", MaxRetries: 0})
	defer down.Close()

	h := &UploadHandler{DB: s.db, RedisClient: down}
	body, ct := multipartBody("document", "duna.epub", corpusFile(t, "epub_acentos.epub"))
	req := httptest.NewRequest("POST", "/upload", body)
	req.Header.Set("Content-Type", ct)
	rec := httptest.NewRecorder()
	h.HandleUpload(rec, req)

	if rec.Code != 200 {
		t.Fatalf("upload with Redis down: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT state FROM jobs`); got != "pending" {
		t.Errorf("the job must wait in the database: %q", got)
	}
}

func TestBulkImport_UsesTheSameChecksAtBatchPriority(t *testing.T) {
	s := newCatalogStack(t)
	dir := filepath.Join(s.storage, "import")
	os.MkdirAll(dir, 0o755)
	for _, name := range []string{"cbz_ltr.cbz", "cbz_rtl.cbz", "epub_acentos.epub", "epub_duplicata.epub", "epub_corrompido.epub", "falso.pdf", "notas.txt"} {
		os.WriteFile(filepath.Join(dir, name), corpusFile(t, name), 0o644)
	}
	os.WriteFile(filepath.Join(dir, "foto.jpg"), []byte("not a supported format"), 0o644)

	rec := s.do(admin, "POST", "/works/bulk-import", "{}")
	if rec.Code != 200 {
		t.Fatalf("bulk import: %d %s", rec.Code, rec.Body.String())
	}
	var r BulkImportResponse
	json.Unmarshal(rec.Body.Bytes(), &r)
	// 7 supported files: 4 accepted, the byte-identical EPUB is a duplicate, 2 are not what they claim.
	if r.Scanned != 7 || r.Enqueued != 4 || r.Duplicates != 1 || r.Errors != 2 {
		t.Errorf("result = %+v, want scanned 7, enqueued 4, duplicates 1, errors 2", r)
	}
	if got := s.scalar(`SELECT count(*) || '/' || min(priority) || '/' || max(priority) FROM jobs`); got != "4/0/0" {
		t.Errorf("jobs (count/min/max priority) = %s, want 4/0/0", got)
	}
	// The originals in the import directory are untouched, and nothing is left in staging.
	for _, f := range s.stored() {
		if strings.Contains(f, ".staging") {
			t.Errorf("staging debris: %s", f)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "cbz_ltr.cbz")); err != nil {
		t.Error("the source file was removed")
	}

	// Running it again finds only duplicates.
	rec = s.do(admin, "POST", "/works/bulk-import", "{}")
	json.Unmarshal(rec.Body.Bytes(), &r)
	if r.Enqueued != 0 || r.Duplicates != 5 {
		t.Errorf("second run = %+v, want everything already stored", r)
	}
}

func TestUpload_HostileNamesAreCleanedNotRefused(t *testing.T) {
	s := newCatalogStack(t)
	for i, name := range []string{
		strings.Repeat("n", 300) + ".txt",
		strings.Repeat("ação ", 80) + ".txt",
		"livro\u202etxt.exe.txt",
		".txt",
		"../../../etc/passwd.txt",
		`C:\Users\Ana\Duna.txt`,
	} {
		content := []byte(fmt.Sprintf("texto número %d, um arquivo diferente de cada vez\n", i))
		if rec := s.upload(admin, name, content); rec.Code != 200 {
			t.Fatalf("%q: %d %s", name, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
	for _, name := range s.stored() {
		base := filepath.Base(name)
		if len(base) > storedNameMax {
			t.Errorf("a stored name of %d bytes: %q", len(base), base)
		}
		if strings.ContainsAny(base, "\n\r\x00\u202e") || strings.Contains(name, "..") || filepath.Dir(name) != "." {
			t.Errorf("a stored name that was not cleaned: %q", name)
		}
	}
	if n := s.scalar(`SELECT count(*) FROM works WHERE char_length(original_title) > 255 OR original_title ~ '[\x00-\x1f]'`); n != "0" {
		t.Errorf("%s titles are too long or have control characters", n)
	}
	for _, want := range []string{"livro txt.exe.txt", "arquivo.txt", "passwd.txt", "Duna.txt"} {
		if s.scalar(`SELECT count(*) FROM works WHERE original_title = '`+want+`'`) != "1" {
			t.Errorf("no work titled %q", want)
		}
	}
	if got := s.scalar(`SELECT count(*) FROM works`); got != "6" {
		t.Errorf("%s works, want 6", got)
	}
}

func TestUpload_AMalformedFormIsABadRequestNotAServerError(t *testing.T) {
	s := newCatalogStack(t)
	// A raw NUL in the file name is not a valid header; the answer is the client's error.
	if rec := s.upload(admin, "livro\x00.txt", []byte("texto\n")); rec.Code != 400 {
		t.Errorf("a NUL in the name: %d, want 400", rec.Code)
	}
	if got := s.counts(); got != "0/0/0" || len(s.stored()) != 0 {
		t.Errorf("a refused upload left something behind: %s %v", got, s.stored())
	}
}

func TestBulkImport_NamesFromADiskThatDoNotFitAreCut(t *testing.T) {
	s := newCatalogStack(t)
	dir := filepath.Join(s.storage, "import")
	os.MkdirAll(dir, 0o755)
	// 250 bytes with accents (the most a file system takes is 255) and a name with a line break.
	long := strings.Repeat("ç", 123) + ".txt"
	weird := "linha1\nlinha2.txt"
	for i, name := range []string{long, weird} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(fmt.Sprintf("texto %d\n", i)), 0o644); err != nil {
			t.Skip("this file system does not take the name: ", err)
		}
	}
	rec := s.do(admin, "POST", "/works/bulk-import", "{}")
	var r BulkImportResponse
	json.Unmarshal(rec.Body.Bytes(), &r)
	if rec.Code != 200 || r.Enqueued != 2 || r.Errors != 0 {
		t.Fatalf("bulk import: %d %+v", rec.Code, r)
	}
	for _, name := range s.stored() {
		if strings.HasPrefix(name, "import") {
			continue
		}
		if len(filepath.Base(name)) > storedNameMax || strings.ContainsAny(name, "\n") {
			t.Errorf("a stored name that does not fit or was not cleaned: %q", name)
		}
	}
	if got := s.scalar(`SELECT count(*) FROM works WHERE original_title = 'linha1 linha2.txt'`); got != "1" {
		t.Errorf("the title with a line break was not cleaned")
	}
}

// A text in Windows-1252, as an old .txt is saved.
func cp1252Text() []byte {
	return []byte("Cora\xe7\xe3o, a\xe7\xe3o e emo\xe7\xe3o: \x93aspas\x94 e travess\xe3o \x97 tamb\xe9m \x805.\r\nSegunda linha.\n")
}

const cp1252TextInUTF8 = "Coração, ação e emoção: “aspas” e travessão — também €5.\r\nSegunda linha.\n"

func utf16LEWithBOM(s string) []byte {
	out := []byte{0xff, 0xfe}
	for _, r := range s {
		out = append(out, byte(r), byte(r>>8)) // the test text is all in the first plane
	}
	return out
}

func TestUpload_TextInAnotherEncodingIsStoredAsUTF8AndSaysSo(t *testing.T) {
	s := newCatalogStack(t)
	for name, tc := range map[string]struct {
		file, from string
		content    []byte
		want       string // what is stored; "" for "not compared"
	}{
		"windows-1252": {"antigo.txt", "Windows-1252", cp1252Text(), cp1252TextInUTF8},
		"markdown":     {"antigo.md", "Windows-1252", cp1252Text()[:20], ""},
		"UTF-16 LE":    {"bloco.txt", "UTF-16 LE", utf16LEWithBOM("Olá, mundo — em UTF-16.\r\n"), "Olá, mundo — em UTF-16.\r\n"},
	} {
		rec := s.upload(admin, tc.file, tc.content)
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", name, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
		var resp struct {
			WorkID        int    `json:"work_id"`
			ConvertedFrom string `json:"converted_from"`
		}
		json.Unmarshal(rec.Body.Bytes(), &resp)
		if resp.ConvertedFrom != tc.from {
			t.Errorf("%s: converted_from = %q, want %q", name, resp.ConvertedFrom, tc.from)
		}
		var stored string
		var size int64
		var sum string
		s.db.QueryRow(`SELECT l.path, f.size_bytes, f.sha256 FROM storage_locations l JOIN files f ON f.id = l.file_id
			JOIN editions e ON e.id = f.edition_id WHERE e.work_id = $1`, resp.WorkID).Scan(&stored, &size, &sum)
		bytesOnDisk, err := os.ReadFile(filepath.Join(s.storage, stored))
		if err != nil {
			t.Fatal(err)
		}
		if !utf8.Valid(bytesOnDisk) || bytes.ContainsRune(bytesOnDisk, 0xfeff) || int64(len(bytesOnDisk)) != size {
			t.Errorf("%s: the stored file is not clean UTF-8 of the size recorded (%d, recorded %d)", name, len(bytesOnDisk), size)
		}
		if h := sha256.Sum256(bytesOnDisk); hex.EncodeToString(h[:]) != sum {
			t.Errorf("%s: the recorded hash is not the hash of the stored bytes", name)
		}
		if tc.want != "" && string(bytesOnDisk) != tc.want {
			t.Errorf("%s: stored %q, want %q", name, bytesOnDisk, tc.want)
		}
	}
	// The same original again is the same file: it converts to the same bytes and is found as a duplicate.
	if rec := s.upload(admin, "outra-copia.txt", cp1252Text()); rec.Code != 409 {
		t.Errorf("the same text again: %d, want 409", rec.Code)
	}
	// And the same text already in UTF-8 is the same file too.
	if rec := s.upload(admin, "em-utf8.txt", []byte(cp1252TextInUTF8)); rec.Code != 409 {
		t.Errorf("the text in UTF-8: %d, want 409", rec.Code)
	}
}

func TestUpload_UTF8TextIsNotTouchedAndDoesNotSayItWasConverted(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.upload(admin, "novo.txt", []byte(cp1252TextInUTF8))
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "converted_from") {
		t.Fatalf("%d %s", rec.Code, rec.Body.String())
	}
	var stored string
	s.db.QueryRow(`SELECT path FROM storage_locations`).Scan(&stored)
	if got, _ := os.ReadFile(filepath.Join(s.storage, stored)); string(got) != cp1252TextInUTF8 {
		t.Errorf("a UTF-8 file changed: %q", got)
	}
}

func TestUpload_TextThatCannotBeToldIsRefusedWithWhatToDo(t *testing.T) {
	s := newCatalogStack(t)
	for name, content := range map[string][]byte{
		"a byte windows-1252 does not have": append(cp1252Text(), 0x81),
		"binary":                            []byte("abc\x00\x01\x02def"),
		"UTF-16 without a byte order mark":  []byte("a\x00b\x00c\x00"),
	} {
		rec := s.upload(admin, "x"+name+".txt", content)
		if rec.Code != 415 || !strings.Contains(rec.Body.String(), "not UTF-8 text") {
			t.Errorf("%s: %d %s", name, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
	if got := s.counts(); got != "0/0/0" || len(s.stored()) != 0 {
		t.Errorf("a refused text left something behind: %s %v", got, s.stored())
	}
}

func TestBulkImport_AMovedTextInAnotherEncodingGoesOnceItIsSafelyCopied(t *testing.T) {
	s := newCatalogStack(t)
	dir := t.TempDir()
	s.addRoot(admin, dir)
	original := filepath.Join(dir, "antigo.txt")
	os.WriteFile(original, cp1252Text(), 0o644)
	rec := s.do(admin, "POST", "/works/bulk-import", fmt.Sprintf(`{"directory":%q,"removeOriginals":true}`, dir))
	var r BulkImportResponse
	json.Unmarshal(rec.Body.Bytes(), &r)
	if rec.Code != 200 || r.Enqueued != 1 || r.Converted != 1 || r.OriginalsRemoved != 1 || r.CleanupPending != 0 {
		t.Fatalf("bulk import: %d %+v", rec.Code, r)
	}
	if _, err := os.Stat(original); !os.IsNotExist(err) {
		t.Error("the original was kept although its content was copied (as UTF-8)")
	}
}

func TestUpload_AnEPUBWithDRMIsRefusedWithTheReasonAndLeavesNothing(t *testing.T) {
	s := newCatalogStack(t)
	drm := zipOf(t, map[string]string{
		"mimetype": "application/epub+zip",
		"META-INF/encryption.xml": `<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><EncryptedData xmlns="http://www.w3.org/2001/04/xmlenc#">` +
			`<EncryptionMethod Algorithm="http://www.w3.org/2001/04/xmlenc#aes128-cbc"/></EncryptedData></encryption>`,
	})
	rec := s.upload(admin, "kindle.epub", drm)
	if rec.Code != 415 || !strings.Contains(rec.Body.String(), "protected by DRM") {
		t.Fatalf("%d %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	if got := s.counts(); got != "0/0/0" || len(s.stored()) != 0 {
		t.Errorf("a refused EPUB left something behind: %s %v", got, s.stored())
	}
	// The same book with only its fonts obfuscated is an ordinary one.
	fonts := zipOf(t, map[string]string{
		"mimetype": "application/epub+zip",
		"META-INF/encryption.xml": `<encryption xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><EncryptedData xmlns="http://www.w3.org/2001/04/xmlenc#">` +
			`<EncryptionMethod Algorithm="http://www.idpf.org/2008/embedding"/></EncryptedData></encryption>`,
	})
	if rec := s.upload(admin, "editora.epub", fonts); rec.Code != 200 {
		t.Errorf("obfuscated fonts are not DRM: %d %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
}
