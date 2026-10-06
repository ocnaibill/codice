package handlers

import (
	"encoding/json"
	"testing"
)

type perfView struct {
	Values     map[string]int `json:"values"`
	Defaults   map[string]int `json:"defaults"`
	Overridden []string       `json:"overridden"`
	Limits     map[string]struct {
		Min int `json:"min"`
		Max int `json:"max"`
	} `json:"limits"`
	Machine struct {
		Cores       int   `json:"cores"`
		MemoryBytes int64 `json:"memoryBytes"`
	} `json:"machine"`
	Warnings []string `json:"warnings"`
	OCR      struct {
		Reported bool `json:"reported"`
		Pages    int  `json:"pages"`
		Threads  int  `json:"threads"`
	} `json:"ocr"`
}

func (s *catalogStack) perf() perfView {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/performance", "")
	if rec.Code != 200 {
		s.t.Fatalf("GET /admin/performance: %d %s", rec.Code, rec.Body.String())
	}
	var v perfView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		s.t.Fatal(err)
	}
	return v
}

func TestPerformance_ShowsTheDefaultsTheLimitsAndTheMachine(t *testing.T) {
	s := newCatalogStack(t)
	v := s.perf()
	want := map[string]int{"catalogReads": 8, "dedupeJobs": 1, "ocrPages": 1, "ocrThreads": 1}
	for k, n := range want {
		if v.Values[k] != n || v.Defaults[k] != n {
			t.Errorf("%s: value %d, default %d, want %d", k, v.Values[k], v.Defaults[k], n)
		}
	}
	if len(v.Overridden) != 0 {
		t.Errorf("nothing was chosen, overridden = %v", v.Overridden)
	}
	if v.Limits["ocrPages"].Max != 8 || v.Limits["dedupeJobs"].Max != 4 || v.Limits["catalogReads"].Max != 20 || v.Limits["ocrPages"].Min != 1 {
		t.Errorf("limits = %+v", v.Limits)
	}
	if v.Machine.Cores != 8 || v.Machine.MemoryBytes != 16<<30 {
		t.Errorf("machine = %+v", v.Machine)
	}
	if v.Warnings == nil || len(v.Warnings) != 0 {
		t.Errorf("warnings = %v, want an empty list (not null)", v.Warnings)
	}
	if v.OCR.Reported {
		t.Error("no OCR service reported, but it says it did")
	}
}

func TestPerformance_AChangeIsStoredAppliedAtOnceAndAudited(t *testing.T) {
	s := newCatalogStack(t)
	rec := s.do(admin, "PUT", "/admin/performance", `{"ocrPages":3,"catalogReads":12}`)
	if rec.Code != 200 {
		t.Fatalf("PUT: %d %s", rec.Code, rec.Body.String())
	}
	var v perfView
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Values["ocrPages"] != 3 || v.Values["catalogReads"] != 12 || v.Values["ocrThreads"] != 1 {
		t.Errorf("answer values = %v", v.Values)
	}
	if len(v.Overridden) != 2 {
		t.Errorf("overridden = %v, want the two that were chosen", v.Overridden)
	}
	if got := s.perf().Values["ocrPages"]; got != 3 {
		t.Errorf("the next read says %d, want 3", got)
	}
	// applied in the process that follows it, at once
	if s.tuning.Get("catalogReads") != 12 || s.tuning.Get("ocrPages") != 3 {
		t.Errorf("live values: catalogReads=%d ocrPages=%d", s.tuning.Get("catalogReads"), s.tuning.Get("ocrPages"))
	}
	if !s.tuning.Overridden("catalogReads") || s.tuning.Overridden("dedupeJobs") {
		t.Error("Overridden says the wrong fields")
	}
	if n := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'performance.update'`); n != "1" {
		t.Errorf("audit entries = %s, want 1", n)
	}
	// the one who acted is recorded
	if who := s.scalar(`SELECT actor_id::text FROM audit_log WHERE action = 'performance.update'`); who != idAdmin {
		t.Errorf("audited actor = %s, want the one who acted", who)
	}
}

func TestPerformance_NullGoesBackToTheDefault(t *testing.T) {
	s := newCatalogStack(t)
	s.do(admin, "PUT", "/admin/performance", `{"ocrPages":3,"dedupeJobs":2}`)
	rec := s.do(admin, "PUT", "/admin/performance", `{"ocrPages":null}`)
	if rec.Code != 200 {
		t.Fatalf("PUT null: %d %s", rec.Code, rec.Body.String())
	}
	v := s.perf()
	if v.Values["ocrPages"] != 1 || v.Values["dedupeJobs"] != 2 {
		t.Errorf("values = %v, want ocrPages back to 1 and dedupeJobs kept at 2", v.Values)
	}
	if len(v.Overridden) != 1 || v.Overridden[0] != "dedupeJobs" {
		t.Errorf("overridden = %v", v.Overridden)
	}
	if s.tuning.Get("ocrPages") != 1 {
		t.Errorf("live ocrPages = %d, want the default again", s.tuning.Get("ocrPages"))
	}
}

func TestPerformance_RefusesWhatIsWrongAndChangesNothing(t *testing.T) {
	s := newCatalogStack(t)
	s.do(admin, "PUT", "/admin/performance", `{"ocrPages":2}`)
	bad := []string{
		`{"ocrPages":0}`, `{"ocrPages":9}`, `{"catalogReads":21}`, `{"dedupeJobs":5}`, `{"ocrThreads":-1}`,
		`{"ocrPages":2.5}`, `{"ocrPages":"3"}`, `{"nope":1}`, `{}`, `[]`, `not json`, ``, `{"ocrPages":1,"nope":1}`,
	}
	for _, body := range bad {
		if rec := s.do(admin, "PUT", "/admin/performance", body); rec.Code != 400 {
			t.Errorf("PUT %q = %d, want 400", body, rec.Code)
		}
	}
	if v := s.perf(); v.Values["ocrPages"] != 2 || len(v.Overridden) != 1 {
		t.Errorf("a refused change changed something: %+v", v.Values)
	}
	if n := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'performance.update'`); n != "1" {
		t.Errorf("audit entries = %s, want 1 (only the good change)", n)
	}
}

func TestPerformance_WarnsWhenItAsksMoreThanTheMachineHas(t *testing.T) {
	s := newCatalogStack(t) // 8 cores, 16 GiB
	s.do(admin, "PUT", "/admin/performance", `{"ocrPages":8,"ocrThreads":2}`)
	warnings := s.perf().Warnings
	if len(warnings) != 1 || warnings[0] != "ocrCores" {
		t.Errorf("warnings = %v, want [ocrCores]: 8 pages of 2 cores is 16 cores on a machine of 8", warnings)
	}
}

func TestPerformance_ShowsWhatTheOCRServiceSaysItUses(t *testing.T) {
	s := newCatalogStack(t)
	s.exec(`INSERT INTO settings (key, value) VALUES ('ocr.worker', '{"engine":"tesseract","pages":2,"threads":1,"state":"idle"}')`)
	v := s.perf()
	if !v.OCR.Reported || v.OCR.Pages != 2 || v.OCR.Threads != 1 {
		t.Errorf("ocr = %+v", v.OCR)
	}
	s.exec(`UPDATE settings SET updated_at = now() - interval '5 minutes' WHERE key = 'ocr.worker'`)
	if s.perf().OCR.Reported {
		t.Error("an OCR service that stopped reporting is still shown as reporting")
	}
	s.exec(`UPDATE settings SET value = '{"engine":"tesseract","state":"idle"}', updated_at = now() WHERE key = 'ocr.worker'`)
	if s.perf().OCR.Reported {
		t.Error("an OCR service of an older version, that does not say pages and threads, is shown as reporting them")
	}
}
