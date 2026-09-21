package handlers

import (
	"encoding/json"
	"testing"
)

func TestEmbeddingsAdmin_DefaultsOffRequiresALiveWorkerAndCanBeDisabledAgain(t *testing.T) {
	s := newCatalogStack(t)
	read := func() map[string]any {
		rec := s.do(admin, "GET", "/admin/embeddings", "")
		if rec.Code != 200 {
			t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
		}
		var body map[string]any
		json.Unmarshal(rec.Body.Bytes(), &body)
		return body
	}
	if state := read(); state["enabled"] != false || state["available"] != false {
		t.Fatalf("default: %+v", state)
	}
	if rec := s.do(admin, "PUT", "/admin/embeddings", `{"enabled":true}`); rec.Code != 409 {
		t.Fatalf("enabled without worker: %d %s", rec.Code, rec.Body.String())
	}
	s.exec(`INSERT INTO settings (key, value) VALUES ('embeddings.worker',
		'{"provider":"sentence-transformers","model":"sentence-transformers/LaBSE","state":"idle","error":""}')`)
	if rec := s.do(admin, "PUT", "/admin/embeddings", `{"enabled":true}`); rec.Code != 200 {
		t.Fatalf("enable: %d %s", rec.Code, rec.Body.String())
	}
	if state := read(); state["enabled"] != true || state["available"] != true || state["state"] != "idle" {
		t.Fatalf("enabled: %+v", state)
	}
	if rec := s.do(admin, "PUT", "/admin/embeddings", `{"enabled":false}`); rec.Code != 200 {
		t.Fatalf("disable: %d %s", rec.Code, rec.Body.String())
	}
	if got := s.scalar(`SELECT count(*) FROM audit_log WHERE action = 'embeddings.policy'`); got != "2" {
		t.Fatalf("audit entries: %s", got)
	}
}
