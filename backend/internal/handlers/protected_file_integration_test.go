package handlers

import (
	"encoding/json"
	"strconv"
	"testing"
)

func TestProtectedFile_TheDetailSaysWhichFilesAskForAPassword(t *testing.T) {
	s := newCatalogStack(t)
	locked := s.addWork("Trancado", "Autor", "trancado.pdf", "pdf")
	plain := s.addWork("Aberto", "Autor", "aberto.pdf", "pdf")
	ed := int(s.primaryEdition(locked))
	other := s.addFile(ed, "epub", "trancado.epub", "managed")
	s.exec(`UPDATE files SET protected = TRUE WHERE id = $1`, s.primaryFile(locked))

	w, code := s.detail(ana, locked)
	if code != 200 || len(w.Editions) != 1 || len(w.Editions[0].Files) != 2 {
		t.Fatalf("detail: %d %+v", code, w.Editions)
	}
	byID := map[int64]bool{}
	for _, f := range w.Editions[0].Files {
		byID[f.ID] = f.Protected
	}
	if !byID[s.primaryFile(locked)] || byID[other] {
		t.Errorf("protected by file = %v (the epub is %d)", byID, other)
	}
	// A file that asks for nothing says nothing, and the field is not even sent.
	p, _ := s.detail(ana, plain)
	if p.Editions[0].Files[0].Protected {
		t.Errorf("a plain file: %+v", p.Editions[0].Files[0])
	}
	raw := s.do(ana, "GET", "/works/"+strconv.Itoa(plain), "").Body.String()
	var decoded struct {
		Editions []struct{ Files []map[string]any }
	}
	json.Unmarshal([]byte(raw), &decoded)
	if _, has := decoded.Editions[0].Files[0]["protected"]; has {
		t.Errorf("the field is sent for a plain file: %v", decoded.Editions[0].Files[0])
	}
	// It is a fact about the file, for every account.
	if w, _ := s.detail(bob, locked); !w.Editions[0].Files[0].Protected && !w.Editions[0].Files[1].Protected {
		t.Error("bob does not see it")
	}
}
