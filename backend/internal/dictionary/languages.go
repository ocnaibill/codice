package dictionary

import (
	"context"
	"database/sql"
	"sort"

	"github.com/lib/pq"
)

// Languages is what the installed dictionaries can be asked in, for the card to offer only that: the languages a word can
// be looked up in (the ones the packages that are ready have entries of, or find by the translations they list), and the
// languages the definitions can be had in (the editions of those packages).
type Languages struct {
	Words       []string `json:"words"`
	Definitions []string `json:"definitions"`
}

// InstalledLanguages reads them from the packages that are ready, and nothing else.
func InstalledLanguages(ctx context.Context, db *sql.DB) (Languages, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, languages, link_languages FROM dictionary_packages WHERE state = 'ready'`)
	if err != nil {
		return Languages{}, err
	}
	defer rows.Close()
	words, definitions := map[string]bool{}, map[string]bool{}
	for rows.Next() {
		var id string
		var own, linked pq.StringArray
		if err := rows.Scan(&id, &own, &linked); err != nil {
			return Languages{}, err
		}
		for _, l := range append(own, linked...) {
			words[l] = true
		}
		if p, ok := Find(id); ok {
			definitions[p.Language()] = true
		}
	}
	if err := rows.Err(); err != nil {
		return Languages{}, err
	}
	return Languages{Words: keys(words), Definitions: keys(definitions)}, nil
}

func keys(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
