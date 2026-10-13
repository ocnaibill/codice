package handlers

import (
	"encoding/json"
	"strings"
	"testing"
)

type modelsState struct {
	Controls map[string]string `json:"controls"`
	Services map[string]bool   `json:"services"`
	Models   []ModelEntry      `json:"models"`
	ComicOcr struct {
		OtherModelPages int `json:"otherModelPages"`
		FailedPages     int `json:"failedPages"`
	} `json:"comicOcr"`
}

func (s *catalogStack) models() modelsState {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/models", "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /admin/models: %d %s", rec.Code, rec.Body.String())
	}
	var out modelsState
	json.Unmarshal(rec.Body.Bytes(), &out)
	return out
}

func (m modelsState) entry(id string) ModelEntry {
	for _, e := range m.Models {
		if e.ID == id {
			return e
		}
	}
	return ModelEntry{}
}

func (s *catalogStack) beat(key, value string) {
	s.t.Helper()
	s.exec(`INSERT INTO settings (key, value) VALUES ($1, $2::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`, key, value)
}

func TestModels_NothingIsChosenAtFirstAndEveryModelIsListedWithItsKind(t *testing.T) {
	s := newCatalogStack(t)
	m := s.models()
	if m.Controls["semantic"] != "none" || m.Controls["comicOcr"] != "none" {
		t.Fatalf("controls: %+v", m.Controls)
	}
	if len(m.Models) != 4 {
		t.Fatalf("models: %+v", m.Models)
	}
	kinds := map[string]int{}
	for _, e := range m.Models {
		kinds[e.Kind]++
		if e.State != "off" || e.Selected || e.InUse || e.SizeMB <= 0 || e.Name == "" || e.Usage.Unit == "" {
			t.Errorf("a model nobody chose: %+v", e)
		}
	}
	if kinds["semantic"] != 2 || kinds["comic_ocr"] != 2 {
		t.Errorf("kinds: %v", kinds)
	}
}

func TestModels_TheReadingOfComicsNeedsItsServiceAndIsKeptWithItsAudit(t *testing.T) {
	s := newCatalogStack(t)
	// With no service running the choice is refused, as it is for the semantic matching.
	if rec := s.do(admin, "PUT", "/admin/models/comic-ocr", `{"model":"mobile"}`); rec.Code != 409 {
		t.Fatalf("without the service: %d %s", rec.Code, rec.Body.String())
	}
	// "none" needs no service: it is how it is turned off.
	if rec := s.do(admin, "PUT", "/admin/models/comic-ocr", `{"model":"none"}`); rec.Code != 200 {
		t.Fatalf("none: %d", rec.Code)
	}
	if rec := s.do(admin, "PUT", "/admin/models/comic-ocr", `{"model":"gigante"}`); rec.Code != 400 {
		t.Fatalf("unknown model: %d", rec.Code)
	}
	s.beat("comic_ocr.worker", `{"model":"mobile","state":"ready","progress":100,"pages":42,"error":""}`)
	rec := s.do(admin, "PUT", "/admin/models/comic-ocr", `{"model":"mobile"}`)
	if rec.Code != 200 {
		t.Fatalf("with the service: %d %s", rec.Code, rec.Body.String())
	}
	m := s.models()
	got := m.entry("mobile")
	if m.Controls["comicOcr"] != "mobile" || !got.Selected || got.State != "ready" || !got.InUse || got.Usage.Count != 42 || got.Usage.Unit != "páginas lidas" {
		t.Errorf("mobile: %+v controls %+v", got, m.Controls)
	}
	if other := m.entry("server"); other.Selected || other.State != "off" || other.Usage.Count != 0 {
		t.Errorf("server: %+v", other)
	}
	if n := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'models.comic_ocr'`); n != "2" {
		t.Errorf("audit entries = %s", n)
	}
}

func TestModels_AChosenModelSaysWhereItIsInItsDownload(t *testing.T) {
	s := newCatalogStack(t)
	s.beat("comic_ocr.worker", `{"model":"server","state":"preparing","progress":37,"pages":0,"error":""}`)
	s.do(admin, "PUT", "/admin/models/comic-ocr", `{"model":"server"}`)
	e := s.models().entry("server")
	if e.State != "downloading" || e.Progress == nil || *e.Progress != 37 || e.InUse {
		t.Errorf("downloading: %+v", e)
	}
	// The service has the other model loaded: the chosen one is being prepared and has no percentage of its own.
	s.beat("comic_ocr.worker", `{"model":"mobile","state":"ready","progress":100,"pages":5,"error":""}`)
	e = s.models().entry("server")
	if e.State != "preparing" || e.Progress != nil || e.InUse {
		t.Errorf("the other one is loaded: %+v", e)
	}
	// A failure is said by the model that is chosen and not by the others.
	s.beat("comic_ocr.worker", `{"model":"server","state":"error","progress":100,"pages":0,"error":"sem memória"}`)
	m := s.models()
	if e := m.entry("server"); e.State != "error" || e.Error != "sem memória" {
		t.Errorf("error: %+v", e)
	}
	if e := m.entry("mobile"); e.Error != "" || e.State != "off" {
		t.Errorf("not chosen: %+v", e)
	}
}

func TestModels_ASilentServiceIsMissingAndTheSemanticModelsReportTheirFiles(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO settings (key, value) VALUES ('equivalence.embeddings', '{"enabled":true,"model":"intfloat/multilingual-e5-small"}')`)
	m := s.models()
	if e := m.entry("intfloat/multilingual-e5-small"); e.State != "service_missing" || !e.Selected || m.Controls["semantic"] != "intfloat/multilingual-e5-small" {
		t.Fatalf("no service: %+v", e)
	}
	s.beat("embeddings.worker", `{"provider":"sentence-transformers","model":"intfloat/multilingual-e5-small","state":"preparing","progress":64,"error":""}`)
	e := s.models().entry("intfloat/multilingual-e5-small")
	if e.State != "downloading" || e.Progress == nil || *e.Progress != 64 {
		t.Errorf("downloading: %+v", e)
	}
	s.beat("embeddings.worker", `{"provider":"sentence-transformers","model":"intfloat/multilingual-e5-small","state":"ready","error":""}`)
	if e := s.models().entry("intfloat/multilingual-e5-small"); e.State != "ready" || !e.InUse || e.Progress != nil || !strings.Contains(e.Usage.Unit, "arquivos") {
		t.Errorf("ready: %+v", e)
	}
	// A service that has not spoken for a while is taken to be gone.
	s.exec(`UPDATE settings SET updated_at = now() - interval '10 minutes' WHERE key = 'embeddings.worker'`)
	if e := s.models().entry("intfloat/multilingual-e5-small"); e.State != "service_missing" {
		t.Errorf("silent: %+v", e)
	}
}

type serviceList struct {
	Services []ServiceStatus `json:"services"`
}

func (s *catalogStack) services() map[string]ServiceStatus {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/services", "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /admin/services: %d", rec.Code)
	}
	var out serviceList
	json.Unmarshal(rec.Body.Bytes(), &out)
	byID := map[string]ServiceStatus{}
	for _, v := range out.Services {
		byID[v.ID] = v
	}
	return byID
}

func TestServices_EachPartSaysWhetherItIsWorkingAndWhatIsOffIsNotADefect(t *testing.T) {
	s := newCatalogStack(t)
	got := s.services()
	if got["database"].Status != "ok" || got["redis"].Status != "off" {
		t.Errorf("database and redis: %+v %+v", got["database"], got["redis"])
	}
	// The worker has not said anything yet.
	if got["jobs"].Status != "down" {
		t.Errorf("jobs: %+v", got["jobs"])
	}
	// Nothing turned on: OCR, comics and semantic matching are off, which is not a defect.
	for _, id := range []string{"ocr", "comic_ocr", "semantic"} {
		if got[id].Status != "off" || got[id].Detail == "" {
			t.Errorf("%s: %+v", id, got[id])
		}
	}
	s.beat("worker.jobs", `{"state":"idle"}`)
	s.exec(`INSERT INTO settings (key, value) VALUES ('ocr', '{"enabled":true,"language":"por"}')`)
	s.exec(`INSERT INTO settings (key, value) VALUES ('comic_ocr', '{"model":"mobile"}')`)
	s.exec(`INSERT INTO settings (key, value) VALUES ('equivalence.embeddings', '{"enabled":true,"model":"x"}')`)
	got = s.services()
	if got["jobs"].Status != "ok" {
		t.Errorf("jobs with a beat: %+v", got["jobs"])
	}
	// Turned on and silent: down, with what to do.
	for _, id := range []string{"ocr", "comic_ocr", "semantic"} {
		if got[id].Status != "down" || !strings.Contains(got[id].Detail, "não está em execução") {
			t.Errorf("%s silent: %+v", id, got[id])
		}
	}
	s.beat("ocr.worker", `{"engine":"tesseract","state":"idle","languages":["por"],"error":""}`)
	s.beat("comic_ocr.worker", `{"model":"mobile","state":"error","error":"modelo corrompido"}`)
	s.beat("embeddings.worker", `{"model":"x","state":"ready","error":""}`)
	got = s.services()
	if got["ocr"].Status != "ok" || got["semantic"].Status != "ok" {
		t.Errorf("working: %+v %+v", got["ocr"], got["semantic"])
	}
	if c := got["comic_ocr"]; c.Status != "error" || c.Detail != "modelo corrompido" {
		t.Errorf("comic_ocr error: %+v", c)
	}
}

func TestModels_TheSemanticModelsCountTheFilesPreparedWithThem(t *testing.T) {
	s := newCatalogStack(t)
	duna := s.addWork("Duna", "Frank Herbert", "duna.epub", "epub")
	file := s.primaryFile(duna)
	s.exec(`INSERT INTO text_embedding_status (file_id, generation, provider, model, revision, preprocessing_version, segment_count)
		VALUES ($1, 1, 'sentence-transformers', 'intfloat/multilingual-e5-small', 'default', 1, 10)`, file)
	m := s.models()
	if e := m.entry("intfloat/multilingual-e5-small"); e.Usage.Count != 1 {
		t.Errorf("e5: %+v", e)
	}
	if e := m.entry("sentence-transformers/LaBSE"); e.Usage.Count != 0 {
		t.Errorf("LaBSE counted the files of the other: %+v", e)
	}
}

func TestModels_AModelThatIsNotTheOneLoadedShowsNoUsageOfItsOwn(t *testing.T) {
	s := newCatalogStack(t)
	s.beat("comic_ocr.worker", `{"model":"mobile","state":"ready","progress":100,"pages":42,"error":""}`)
	m := s.models()
	if e := m.entry("mobile"); e.Usage.Count != 42 {
		t.Errorf("mobile: %+v", e)
	}
	if e := m.entry("server"); e.Usage.Count != 0 {
		t.Errorf("the pages of one are not the pages of the other: %+v", e)
	}
}

func TestModels_AChoiceThatIsNotOneOfTheModelsCountsAsNone(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO settings (key, value) VALUES ('comic_ocr', '{"model":"gigante"}')`)
	m := s.models()
	if m.Controls["comicOcr"] != "none" || m.entry("mobile").Selected || m.entry("server").Selected {
		t.Errorf("controls: %+v", m.Controls)
	}
}
