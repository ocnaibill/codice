package handlers

import (
	"bytes"
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
	names := s.scalar(`SELECT count(DISTINCT file_path) FROM works`)
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
