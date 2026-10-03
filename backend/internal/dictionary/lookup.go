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
	// The bridge through English: how many English words it follows from a word, and how many words it says for each.
	maxBridgeEnglish = 6
	maxBridgeWords   = 8
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

// Rank orders what a lookup found by whose definitions they are: first the ones in the language the person wants to read
// them in, then the ones in the language of the word, then the rest. It keeps the order inside a package (the entries of
// a word, the lemma it comes from, what it translates).
func rank(items []Item, prefer, lang string) {
	edition := func(it Item) int {
		p, _ := Find(it.Entry.Package)
		switch {
		case prefer != "" && p.Edition == prefer:
			return 0
		case p.Edition == lang:
			return 1
		}
		return 2
	}
	// A package's items stay together: the ones of two packages that rank the same do not mix.
	sort.SliceStable(items, func(i, j int) bool {
		if a, b := edition(items[i]), edition(items[j]); a != b {
			return a < b
		}
		return items[i].Entry.Package < items[j].Entry.Package
	})
}

// Candidate is one way across the bridge: the English word the word is listed under, and the words of the language the
// reader wants that the dictionaries list for that English word. It is a candidate and not an answer: a word of English
// has many meanings and the bridge does not know which one the word has.
type Candidate struct {
	English string   `json:"english"`
	Words   []string `json:"words"`
}

// Bridge is what the lookup tried through English when the two languages were not linked directly (plan B, DEC-117). It is
// always labelled as what it is: an approximation, by way of English, with candidates.
type Bridge struct {
	Via  string `json:"via"`
	From string `json:"from"`
	To   string `json:"to"`
	// Available says whether the English package is installed: without it the bridge goes only through what the other
	// packages list of English, and the card says the English dictionary would make it better.
	Available  bool        `json:"available"`
	Candidates []Candidate `json:"candidates"`
}

// Result is what a lookup found of a word in a language. Installed says whether there is a dictionary at all: without one
// there is nothing to find, and the card says so.
type Result struct {
	Word      string   `json:"word"`
	Lang      string   `json:"lang"`
	Prefer    string   `json:"prefer,omitempty"`
	Installed bool     `json:"installed"`
	Items     []Item   `json:"items"`
	Sources   []Source `json:"sources"`
	// Bridge is there when the dictionaries do not link the word's language to the one the person wants, and the lookup
	// tried through English.
	Bridge *Bridge `json:"bridge,omitempty"`
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
// translation. What is found first is what the word is, then what it comes from, then what it translates. `prefer` is the
// language the person wants the definitions in ("" for none): the ones in it come first.
func Lookup(ctx context.Context, db *sql.DB, lang, prefer, word string) (Result, error) {
	key := Normalize(word)
	if key == "" {
		return Result{}, ErrNoWord
	}
	res := Result{Word: word, Lang: lang, Prefer: prefer, Items: []Item{}, Sources: []Source{}}
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

	rank(res.Items, prefer, lang)
	var extra []string
	if wantsBridge(res.Items, lang, prefer) {
		words := []string{word}
		for _, it := range res.Items {
			words = append(words, it.Entry.Word)
		}
		var err error
		res.Bridge, extra, err = bridge(ctx, db, lang, prefer, unique(words))
		if err != nil {
			return res, err
		}
	}
	res.Sources = sourcesOf(res.Items, extra)
	return res, nil
}

// reaches says whether an item links the word's language to the one the person wants directly: the word is listed under an
// entry of that language, or it is a word of its own language that the dictionary defines in the wanted one or lists a
// translation of into it. An English entry is not that, when neither language is English: the word is listed under it,
// which is the first step of the bridge, and the bridge says so.
func reaches(it Item, lang, prefer string) bool {
	if it.Entry.Lang == prefer {
		return true
	}
	if it.Entry.Lang != lang {
		return false
	}
	if p, ok := Find(it.Entry.Package); ok && p.Edition == prefer {
		return true
	}
	var data struct {
		Translations []struct {
			Lang string `json:"lang"`
		} `json:"translations"`
	}
	if json.Unmarshal(it.Entry.Data, &data) != nil {
		return false
	}
	for _, t := range data.Translations {
		if t.Lang == prefer {
			return true
		}
	}
	return false
}

// wantsBridge says whether English is tried: when the person said which language they want, it is another than the word's,
// neither is English (English as a language is a direct link, not a bridge), and nothing the dictionaries found reaches it.
func wantsBridge(items []Item, lang, prefer string) bool {
	if prefer == "" || prefer == lang || lang == "en" || prefer == "en" {
		return false
	}
	for _, it := range items {
		if reaches(it, lang, prefer) {
			return false
		}
	}
	return true
}

// bridge goes from the word to English and from there to the language the person wants, through what the dictionaries
// list: an English word that lists the word as a translation (or that an entry of the word lists), and the words of the
// wanted language that English word is listed with, from its own translations and from the entries of the wanted language
// that list it. It returns the packages it used, to be credited.
func bridge(ctx context.Context, db *sql.DB, lang, prefer string, words []string) (*Bridge, []string, error) {
	b := &Bridge{Via: "en", From: lang, To: prefer, Candidates: []Candidate{}}
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM dictionary_packages WHERE id = 'wikt-en' AND state = 'ready')`).Scan(&b.Available); err != nil {
		return nil, nil, err
	}
	norms := make([]string, 0, len(words))
	for _, w := range words {
		if n := Normalize(w); n != "" {
			norms = append(norms, n)
		}
	}
	used := map[string]bool{}

	// The English words the word is listed under.
	rows, err := db.QueryContext(ctx, `
		SELECT l.target_word, l.package_id FROM dictionary_links l JOIN dictionary_packages p ON p.id = l.package_id AND p.state = 'ready'
		WHERE l.lang = $1 AND l.norm = ANY($2) AND l.target_lang = 'en'
		UNION ALL
		SELECT l.word, l.package_id FROM dictionary_links l JOIN dictionary_packages p ON p.id = l.package_id AND p.state = 'ready'
		WHERE l.target_lang = $1 AND l.target_word = ANY($3) AND l.lang = 'en'`, lang, pq.Array(norms), pq.Array(words))
	if err != nil {
		return nil, nil, err
	}
	englishBy := map[string][]string{} // English word -> packages that say so
	for rows.Next() {
		var e, pkg string
		if err := rows.Scan(&e, &pkg); err != nil {
			rows.Close()
			return nil, nil, err
		}
		englishBy[e] = append(englishBy[e], pkg)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	rows.Close()
	english := make([]string, 0, len(englishBy))
	for e := range englishBy {
		english = append(english, e)
	}
	// What more than one listing says comes first.
	sort.Slice(english, func(i, j int) bool {
		if a, c := len(englishBy[english[i]]), len(englishBy[english[j]]); a != c {
			return a > c
		}
		return english[i] < english[j]
	})
	if len(english) > maxBridgeEnglish {
		english = english[:maxBridgeEnglish]
	}
	if len(english) == 0 {
		return b, nil, nil
	}
	for _, e := range english {
		for _, pkg := range englishBy[e] {
			used[pkg] = true
		}
	}

	// The words of the wanted language each of them is listed with: the English word's own translations, and the entries of
	// the wanted language that list the English word.
	englishNorms := make([]string, 0, len(english))
	byNorm := map[string][]string{} // what an English word reads as -> the English words that read so
	for _, e := range english {
		n := Normalize(e)
		englishNorms = append(englishNorms, n)
		byNorm[n] = append(byNorm[n], e)
	}
	rows, err = db.QueryContext(ctx, `
		SELECT l.target_word, l.word, l.package_id FROM dictionary_links l JOIN dictionary_packages p ON p.id = l.package_id AND p.state = 'ready'
		WHERE l.target_lang = 'en' AND l.target_word = ANY($1) AND l.lang = $2
		UNION ALL
		SELECT l.word, l.target_word, l.package_id FROM dictionary_links l JOIN dictionary_packages p ON p.id = l.package_id AND p.state = 'ready'
		WHERE l.lang = 'en' AND l.norm = ANY($3) AND l.target_lang = $2`, pq.Array(english), prefer, pq.Array(englishNorms))
	if err != nil {
		return nil, nil, err
	}
	support := map[string]map[string]int{} // English word -> word of the wanted language -> how many listings say so
	said := map[string]map[string]bool{}   // ... and which packages
	for rows.Next() {
		var e, c, pkg string
		if err := rows.Scan(&e, &c, &pkg); err != nil {
			rows.Close()
			return nil, nil, err
		}
		for _, original := range byNorm[Normalize(e)] {
			if support[original] == nil {
				support[original] = map[string]int{}
				said[original] = map[string]bool{}
			}
			support[original][c]++
			said[original][pkg] = true
		}
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	rows.Close()
	for _, e := range english {
		list := make([]string, 0, len(support[e]))
		for c := range support[e] {
			list = append(list, c)
		}
		sort.Slice(list, func(i, j int) bool {
			if a, c := support[e][list[i]], support[e][list[j]]; a != c {
				return a > c
			}
			return list[i] < list[j]
		})
		if len(list) == 0 {
			continue
		}
		if len(list) > maxBridgeWords {
			list = list[:maxBridgeWords]
		}
		b.Candidates = append(b.Candidates, Candidate{English: e, Words: list})
		for pkg := range said[e] {
			used[pkg] = true
		}
	}
	ids := make([]string, 0, len(used))
	for pkg := range used {
		ids = append(ids, pkg)
	}
	sort.Strings(ids)
	return b, ids, nil
}

// entriesOf is the entries of a word of a language, by what it reads as.
func entriesOf(ctx context.Context, db *sql.DB, lang, word string, limit int) ([]Entry, error) {
	rows, err := db.QueryContext(ctx, readyEntries+` WHERE e.lang = $1 AND e.norm = $2 AND e.word = $3 ORDER BY e.id LIMIT $4`, lang, Normalize(word), word, limit)
	if err != nil {
		return nil, err
	}
	return scanEntries(rows)
}

// sourcesOf says where what a lookup found comes from: the packages of the items, then the others the bridge used.
func sourcesOf(items []Item, extra []string) []Source {
	out := []Source{}
	seen := map[string]bool{}
	ids := make([]string, 0, len(items)+len(extra))
	for _, it := range items {
		ids = append(ids, it.Entry.Package)
	}
	ids = append(ids, extra...)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		if p, ok := Find(id); ok {
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
