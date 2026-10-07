package dictionary

import (
	"context"
	"database/sql"
)

// maxOtherLanguages is how many other languages a lookup shows: a word that is a word in a dozen languages is a short word, and
// the card has room for few.
const maxOtherLanguages = 6

// LanguageResult is what the lookup found of a word in one language other than the one it was asked in.
type LanguageResult struct {
	Lang   string `json:"lang"`
	Result Result `json:"result"`
}

// LookupOthers finds a word in the languages other than `exclude` that the installed dictionaries have it in (#183): the person
// selected a word in a book of one language and it is a word of another. Which languages have it is one question of the
// dictionaries (as an entry, as a form of another word, or as a translation that an entry lists); each of them is then looked
// up as the person would have asked, with the definitions first in the language `prefer`. A language that has the word only
// by a form that points to nothing, or whose lookup finds no item, is not shown. The languages come in the order of their
// codes, at most maxOtherLanguages of them.
func LookupOthers(ctx context.Context, db *sql.DB, exclude, prefer, word string) ([]LanguageResult, error) {
	key := Normalize(word)
	if key == "" {
		return nil, ErrNoWord
	}
	rows, err := db.QueryContext(ctx, `
		SELECT lang FROM (
			SELECT e.lang FROM dictionary_entries e JOIN dictionary_packages p ON p.id = e.package_id AND p.state = 'ready' WHERE e.norm = $1
			UNION
			SELECT f.lang FROM dictionary_forms f JOIN dictionary_packages p ON p.id = f.package_id AND p.state = 'ready' WHERE f.norm = $1
			UNION
			SELECT l.lang FROM dictionary_links l JOIN dictionary_packages p ON p.id = l.package_id AND p.state = 'ready' WHERE l.norm = $1
		) found WHERE lang <> $2 ORDER BY lang`, key, exclude)
	if err != nil {
		return nil, err
	}
	var langs []string
	for rows.Next() {
		var lang string
		if err := rows.Scan(&lang); err != nil {
			rows.Close()
			return nil, err
		}
		langs = append(langs, lang)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []LanguageResult{}
	for _, lang := range langs {
		if len(out) >= maxOtherLanguages {
			break
		}
		res, err := Lookup(ctx, db, lang, prefer, word)
		if err != nil {
			return nil, err
		}
		if len(res.Items) == 0 {
			continue
		}
		res.Bridge = nil // the bridge answers a question about the language the person asked in, not about these
		out = append(out, LanguageResult{Lang: lang, Result: res})
	}
	return out, nil
}
