package handlers

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestAbout_SaysTheVersionTheLicenseAndWhereTheSourceIs(t *testing.T) {
	h := &AboutHandler{Version: "6cd17ef", SourceURL: "https://git.example.com/me/codice"}
	rec := httptest.NewRecorder()
	h.Get(rec, httptest.NewRequest("GET", "/about", nil))
	if rec.Code != 200 || rec.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status %d, type %q", rec.Code, rec.Header().Get("Content-Type"))
	}
	var got map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"version":    "6cd17ef",
		"license":    "AGPL-3.0",
		"licenseUrl": "https://www.gnu.org/licenses/agpl-3.0.html",
		"sourceUrl":  "https://git.example.com/me/codice",
	}
	if len(got) != len(want) {
		t.Errorf("fields = %v, want exactly %v: nothing about the instance may be added here (DEC-120)", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}
