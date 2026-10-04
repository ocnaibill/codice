package middleware

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func readAll(t *testing.T, contentType string, size int, limit int64) error {
	t.Helper()
	var err error
	h := LimitBody(limit)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err = io.Copy(io.Discard, r.Body)
	}))
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(strings.Repeat("a", size)))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	h.ServeHTTP(httptest.NewRecorder(), req)
	return err
}

func TestLimitBodyCapsJSON(t *testing.T) {
	if err := readAll(t, "application/json", 100, 100); err != nil {
		t.Fatalf("a body at the limit must pass: %v", err)
	}
	if err := readAll(t, "application/json", 101, 100); err == nil {
		t.Fatal("a body over the limit must fail")
	}
}

func TestLimitBodyCapsWhateverTheContentType(t *testing.T) {
	for _, ct := range []string{"", "text/plain", "application/octet-stream", "multipart/mixed", "not a type;;"} {
		if err := readAll(t, ct, 101, 100); err == nil {
			t.Errorf("content type %q: a body over the limit must fail", ct)
		}
	}
}

func TestLimitBodySkipsUploads(t *testing.T) {
	for _, ct := range []string{"multipart/form-data; boundary=xyz", "Multipart/Form-Data; boundary=xyz"} {
		if err := readAll(t, ct, 1000, 100); err != nil {
			t.Errorf("content type %q: the upload has its own limit: %v", ct, err)
		}
	}
}

func TestLimitBodyAnswersWithoutBody(t *testing.T) {
	h := LimitBody(10)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Body = nil
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rec.Code)
	}
}
