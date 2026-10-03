package dictionary

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"

	"github.com/lib/pq"
)

// ErrNoWord is a lookup of nothing: a word that reads as no letter.
var ErrNoWord = errors.New("the word reads as nothing")

// Limits of what a lookup brings: a word of a common language can have many entries (a noun, a verb, a form of another
// verb), and a card has room for few.
const (
	maxDirect       = 12
	maxLemmas       = 8
	maxTranslations = 10
	maxPerLemma     = 6
)

// Entry is one entry of a dictionary, as it was kept (worker/dictionary.py): the word, its class and the data the card is
// made of (the senses, the forms, a few translations, the pronunciation).
type Entry struct {
	ID      int64           `json:"id"`
	Package string          `json:"package"`
	Lang    string          `json:"lang"`
	Word    string          `json:"word"`
	Pos     string          `json:"pos"`
	Data    json.RawMessage `json:"data"`
}

// Item is one thing a lookup found, and why it is there.
//   - "entry": the word is an entry of the dictionary.
//   - "lemma": the word is a form of this entry's word ("correram" of "correr"); Form is the word as the dictionary knows it
//     (when it is not an entry of its own, Tags say which form it is).
//   - "translation": the word is listed as a translation of this entry's word (a Japanese 走る is listed under "correr");
//     Via is the word as the entry lists it and Sense the meaning it was listed under.
type Item struct {
	Kind  string   `json:"kind"`
	Entry Entry    `json:"entry"`
	Form  string   `json:"form,omitempty"`
	Tags  []string `json:"tags,omitempty"`
	Via   string   `json:"via,omitempty"`
	Sense string   `json:"sense,omitempty"`
}

// Source says where what a lookup found comes from (the license asks for it to be said).
type Source struct {
	Package    string `json:"package"`
	Name       string `json:"name"`
	License    string `json:"license"`
	LicenseURL string `json:"licenseUrl"`
	Source     string `json:"source"`
	SourceURL  string `json:"sourceUrl"`
}

// Result is what a lookup found of a word in a language. Installed says whether there is a dictionary at all: without one
// there is nothing to find, and the card says so.
type Result struct {
	Word      string   `json:"word"`
	Lang      string   `json:"lang"`
	Installed bool     `json:"installed"`
	Items     []Item   `json:"items"`
	Sources   []Source `json:"sources"`
}

// readyEntries is what every query of entries starts from: the entries of the packages that are ready.
const readyEntries = `
	SELECT e.id, e.package_id, e.lang, e.word, e.pos, e.data
	FROM dictionary_entries e JOIN dictionary_packages p ON p.id = e.package_id AND p.state = 'ready'`

func scanEntries(rows *sql.Rows) ([]Entry, error) {
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var data []byte
		if err := rows.Scan(&e.ID, &e.Package, &e.Lang, &e.Word, &e.Pos, &data); err != nil {
			return nil, err
		}
		e.Data = json.RawMessage(data)
		out = append(out, e)
	}
	return out, rows.Err()
}

// formOf says which words an entry is a form of, from its senses.
func formOf(e Entry) []string {
	var data struct {
		Senses []struct {
			FormOf []struct {
				Word string `json:"word"`
			} `json:"form_of"`
		} `json:"senses"`
	}
	if json.Unmarshal(e.Data, &data) != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, s := range data.Senses {
		for _, f := range s.FormOf {
			if f.Word != "" && !seen[f.Word] {
				seen[f.Word] = true
				out = append(out, f.Word)
			}
		}
	}
	return out
}

// Lookup finds a word of a language: the entries it has, the words it is a form of, and the entries that list it as a
// translation. What is found first is what the word is, then what it comes from, then what it translates.
func Lookup(ctx context.Context, db *sql.DB, lang, word string) (Result, error) {
	key := Normalize(word)
	if key == "" {
		return Result{}, ErrNoWord
	}
	res := Result{Word: word, Lang: lang, Items: []Item{}, Sources: []Source{}}
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM dictionary_packages WHERE state = 'ready')`).Scan(&res.Installed); err != nil {
		return res, err
	}
	if !res.Installed {
		return res, nil
	}

	seen := map[int64]bool{}
	add := func(item Item) {
		if !seen[item.Entry.ID] {
			seen[item.Entry.ID] = true
			res.Items = append(res.Items, item)
		}
	}

	rows, err := db.QueryContext(ctx, readyEntries+` WHERE e.lang = $1 AND e.norm = $2 ORDER BY e.id LIMIT $3`, lang, key, maxDirect)
	if err != nil {
		return res, err
	}
	direct, err := scanEntries(rows)
	if err != nil {
		return res, err
	}
	// What the word is on its own comes before what is only a form of another: "livro" the noun before "livro" of livrar.
	sort.SliceStable(direct, func(i, j int) bool { return len(formOf(direct[i])) == 0 && len(formOf(direct[j])) > 0 })
	var lemmaWords []string
	forms := map[string][]string{} // lemma word -> the form as the entry lists it
	for _, e := range direct {
		add(Item{Kind: "entry", Entry: e})
		for _, w := range formOf(e) {
			lemmaWords = append(lemmaWords, w)
			forms[w] = append(forms[w], e.Word)
		}
	}

	// The forms a lemma lists, for a form that has no entry of its own.
	fr, err := db.QueryContext(ctx, `
		SELECT f.lemma, f.form, f.tags FROM dictionary_forms f JOIN dictionary_packages p ON p.id = f.package_id AND p.state = 'ready'
		WHERE f.lang = $1 AND f.norm = $2 ORDER BY f.lemma LIMIT $3`, lang, key, maxLemmas)
	if err != nil {
		return res, err
	}
	listed := map[string][]string{} // lemma word -> tags of the form
	listedForm := map[string]string{}
	for fr.Next() {
		var lemma, form string
		var tags pq.StringArray
		if err := fr.Scan(&lemma, &form, &tags); err != nil {
			fr.Close()
			return res, err
		}
		lemmaWords = append(lemmaWords, lemma)
		listed[lemma] = append(listed[lemma], tags...)
		listedForm[lemma] = form
	}
	if err := fr.Err(); err != nil {
		return res, err
	}
	fr.Close()

	lemmaWords = unique(lemmaWords)
	if len(lemmaWords) > maxLemmas {
		lemmaWords = lemmaWords[:maxLemmas]
	}
	for _, w := range lemmaWords {
		entries, err := entriesOf(ctx, db, lang, w, maxPerLemma)
		if err != nil {
			return res, err
		}
		for _, e := range entries {
			if len(formOf(e)) > 0 { // a lemma that is itself a form of something else is not what the word comes from
				continue
			}
			item := Item{Kind: "lemma", Entry: e, Tags: unique(listed[w])}
			if len(forms[w]) > 0 {
				item.Form = forms[w][0]
			} else {
				item.Form = listedForm[w]
			}
			add(item)
		}
	}

	// The entries that list the word as a translation.
	lr, err := db.QueryContext(ctx, `
		SELECT l.target_lang, l.target_word, l.word, l.sense FROM dictionary_links l JOIN dictionary_packages p ON p.id = l.package_id AND p.state = 'ready'
		WHERE l.lang = $1 AND l.norm = $2 ORDER BY l.target_lang, l.target_word LIMIT $3`, lang, key, maxTranslations*3)
	if err != nil {
		return res, err
	}
	type link struct{ lang, word, via, sense string }
	var links []link
	for lr.Next() {
		var l link
		if err := lr.Scan(&l.lang, &l.word, &l.via, &l.sense); err != nil {
			lr.Close()
			return res, err
		}
		links = append(links, l)
	}
	if err := lr.Err(); err != nil {
		return res, err
	}
	lr.Close()
	translations := 0
	for _, l := range links {
		if translations >= maxTranslations {
			break
		}
		entries, err := entriesOf(ctx, db, l.lang, l.word, maxPerLemma)
		if err != nil {
			return res, err
		}
		for _, e := range entries {
			if len(formOf(e)) > 0 || seen[e.ID] {
				continue
			}
			add(Item{Kind: "translation", Entry: e, Via: l.via, Sense: l.sense})
			translations++
		}
	}

	res.Sources = sourcesOf(res.Items)
	return res, nil
}

// entriesOf is the entries of a word of a language, by what it reads as.
func entriesOf(ctx context.Context, db *sql.DB, lang, word string, limit int) ([]Entry, error) {
	rows, err := db.QueryContext(ctx, readyEntries+` WHERE e.lang = $1 AND e.norm = $2 AND e.word = $3 ORDER BY e.id LIMIT $4`, lang, Normalize(word), word, limit)
	if err != nil {
		return nil, err
	}
	return scanEntries(rows)
}

func sourcesOf(items []Item) []Source {
	out := []Source{}
	seen := map[string]bool{}
	for _, it := range items {
		if seen[it.Entry.Package] {
			continue
		}
		seen[it.Entry.Package] = true
		if p, ok := Find(it.Entry.Package); ok {
			out = append(out, Source{Package: p.ID, Name: p.Name, License: p.License, LicenseURL: p.LicenseURL, Source: p.Source, SourceURL: p.SourceURL})
		}
	}
	return out
}

func unique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
