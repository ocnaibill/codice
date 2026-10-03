package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/ocnaibill/codice/backend/internal/dictionary"
)

// DictionaryLookupHandler looks up a word the reader selected in the dictionaries the owner installed (#109). The word goes
// no further than this server, and nothing is kept of what was looked up.
type DictionaryLookupHandler struct{ DB *sql.DB }

// maxLookupWord is the longest thing looked up: a word, or a few of them (a word with a hyphen, a short expression).
const maxLookupWord = 80

var langCode = regexp.MustCompile(`^[a-z]{2,3}$`)

// Lookup answers GET /dictionary?lang=pt&word=correram. The language is the one the word is in, as a code (pt, pt-BR and
// pt_BR are all pt).
func (h *DictionaryLookupHandler) Lookup(w http.ResponseWriter, r *http.Request) {
	word := strings.TrimSpace(r.URL.Query().Get("word"))
	lang := strings.ToLower(r.URL.Query().Get("lang"))
	if i := strings.IndexAny(lang, "-_"); i >= 0 {
		lang = lang[:i]
	}
	if word == "" || !langCode.MatchString(lang) {
		http.Error(w, "Informe a palavra e o idioma.", http.StatusBadRequest)
		return
	}
	if utf8.RuneCountInString(word) > maxLookupWord {
		http.Error(w, "O trecho é longo demais para procurar no dicionário.", http.StatusBadRequest)
		return
	}
	res, err := dictionary.Lookup(r.Context(), h.DB, lang, word)
	if errors.Is(err, dictionary.ErrNoWord) {
		http.Error(w, "Nenhuma palavra para procurar.", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Println("Error looking up a word:", err)
		http.Error(w, "Error looking up the word", http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(res)
}
