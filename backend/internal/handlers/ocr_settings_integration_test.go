package handlers

import (
	"encoding/json"
	"testing"
)

func (s *catalogStack) ocrState() map[string]any {
	s.t.Helper()
	rec := s.do(admin, "GET", "/admin/ocr/settings", "")
	if rec.Code != 200 {
		s.t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	json.Unmarshal(rec.Body.Bytes(), &body)
	return body
}

func (s *catalogStack) ocrWorker(languages string) {
	s.t.Helper()
	s.exec(`INSERT INTO settings (key, value) VALUES ('ocr.worker', $1::jsonb)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()`,
		`{"engine":"tesseract","version":"5.3.0","languages":`+languages+`,"state":"idle","error":""}`)
}

func TestOCRSettings_DefaultsOffAndNeedsTheEngineRunningToBeTurnedOn(t *testing.T) {
	s := newCatalogStack(t)
	state := s.ocrState()
	if state["enabled"] != false || state["available"] != false || state["language"] != defaultOCRLanguage {
		t.Fatalf("default: %+v", state)
	}
	if langs, _ := state["languages"].([]any); len(langs) != 0 {
		t.Errorf("languages without an engine: %v", state["languages"])
	}
	if rec := s.do(admin, "PUT", "/admin/ocr/settings", `{"enabled":true}`); rec.Code != 409 {
		t.Fatalf("enabling without the engine: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.ocrState()["enabled"]; got != false {
		t.Errorf("a refused request changed the setting: %v", got)
	}

	s.ocrWorker(`["eng","por","spa"]`)
	if rec := s.do(admin, "PUT", "/admin/ocr/settings", `{"enabled":true,"language":"por"}`); rec.Code != 200 {
		t.Fatalf("enable: %d %s", rec.Code, rec.Body.String())
	}
	state = s.ocrState()
	if state["enabled"] != true || state["available"] != true || state["language"] != "por" || state["engine"] != "tesseract" || state["engineVersion"] != "5.3.0" || state["state"] != "idle" {
		t.Errorf("enabled: %+v", state)
	}
	if langs, _ := state["languages"].([]any); len(langs) != 3 {
		t.Errorf("languages: %v", state["languages"])
	}

	// It can be turned off whatever the engine is doing, and it keeps the language chosen.
	s.exec(`DELETE FROM settings WHERE key = 'ocr.worker'`)
	if rec := s.do(admin, "PUT", "/admin/ocr/settings", `{"enabled":false,"language":"por"}`); rec.Code != 200 {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body.String())
	}
	if state := s.ocrState(); state["enabled"] != false || state["language"] != "por" {
		t.Errorf("disabled: %+v", state)
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'ocr.policy'`); got != "2" {
		t.Errorf("audit entries: %s", got)
	}
}

func TestOCRSettings_AnEngineThatStoppedReportingIsNotAvailable(t *testing.T) {
	s := newCatalogStack(t)
	s.ocrWorker(`["eng","por"]`)
	s.exec(`UPDATE settings SET updated_at = now() - interval '5 minutes' WHERE key = 'ocr.worker'`)
	if state := s.ocrState(); state["available"] != false {
		t.Fatalf("stale report: %+v", state)
	}
	if rec := s.do(admin, "PUT", "/admin/ocr/settings", `{"enabled":true}`); rec.Code != 409 {
		t.Errorf("enabling with an engine that is gone: %d", rec.Code)
	}
}

func TestOCRSettings_OnlyALanguageTheEngineHasCanBeChosen(t *testing.T) {
	s := newCatalogStack(t)
	s.ocrWorker(`["eng","por"]`)
	// In this order: the last one accepted is "eng", and what is refused after it must not change that.
	for _, c := range []struct {
		body string
		want int
	}{
		{`{"enabled":true,"language":"por+eng"}`, 200},
		{`{"enabled":true,"language":"eng"}`, 200},
		{`{"enabled":true,"language":"deu"}`, 400}, // the engine has not got it
		{`{"enabled":true,"language":"por+deu"}`, 400},
		{`{"enabled":true,"language":"POR"}`, 400}, // not a code of the engine
		{`{"enabled":true,"language":"por eng"}`, 400},
		{`{"enabled":true,"language":"../x"}`, 400},
		{`{"enabled":true,"language":"por+"}`, 400},
		{`{"enabled":true,"language":"a"}`, 400},
		{`not json`, 400},
	} {
		if rec := s.do(admin, "PUT", "/admin/ocr/settings", c.body); rec.Code != c.want {
			t.Errorf("%s: %d, want %d (%s)", c.body, rec.Code, c.want, rec.Body.String())
		}
	}
	if got := s.ocrState()["language"]; got != "eng" {
		t.Errorf("a refused request must not change the language: %v", got)
	}
}

func TestOCRSettings_WithoutALanguageItIsTheDefaultOne(t *testing.T) {
	s := newCatalogStack(t)
	s.ocrWorker(`["eng","por"]`)
	if rec := s.do(admin, "PUT", "/admin/ocr/settings", `{"enabled":true}`); rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if got := s.ocrState()["language"]; got != defaultOCRLanguage {
		t.Errorf("language: %v", got)
	}
}

func TestOCRSettings_ALanguageThatIsNotACodeIsRefusedEvenWithoutTheEngine(t *testing.T) {
	s := newCatalogStack(t)
	// With no engine to ask, the form of the language is all that can be checked.
	for _, language := range []string{"../x", "POR", "por eng", "por+", "a", "por+eng+por+eng+por"} {
		body := `{"enabled":false,"language":"` + language + `"}`
		if rec := s.do(admin, "PUT", "/admin/ocr/settings", body); rec.Code != 400 {
			t.Errorf("%s: %d, want 400", body, rec.Code)
		}
	}
	if rec := s.do(admin, "PUT", "/admin/ocr/settings", `{"enabled":false,"language":"por+eng"}`); rec.Code != 200 {
		t.Errorf("a code the engine may have, with no engine to say: %d", rec.Code)
	}
}
