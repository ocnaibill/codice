package wiki

import (
	"context"
	"database/sql"
	"errors"
	"sort"
)

// The page of an entity as a reader sees it from their cursor (DEC-178, DEC-179). Only what the cursor reaches is in it: a
// biography part once the part is read, the "Quem é" of the last part read, the names, relations, states and references up to
// the cursor, and the sentences whose every reference is reached.

type SentenceView struct {
	ID   int64  `json:"id"`
	Text string `json:"text"`
	Refs []int  `json:"refs"` // numbers of the references below
}

type Paragraph []SentenceView

type WhoView struct {
	Book       int         `json:"book"`
	Part       string      `json:"part"`
	Label      string      `json:"label"` // the point it was updated at
	Paragraphs []Paragraph `json:"paragraphs"`
}

type BioPart struct {
	Part        string      `json:"part"`
	ChapterFrom int         `json:"chapterFrom"`
	ChapterTo   int         `json:"chapterTo"`
	Paragraphs  []Paragraph `json:"paragraphs"`
}

type BioBook struct {
	Book  int       `json:"book"`
	Title string    `json:"title"`
	Side  bool      `json:"side"`
	Parts []BioPart `json:"parts"`
}

type StateView struct {
	Book    int    `json:"book"`
	Chapter int    `json:"chapter"`
	Label   string `json:"label"`
	Text    string `json:"text"`
}

// ProgressView is the part the reader is in: its biography is not shown yet, its chapters are, one by one.
type ProgressView struct {
	Book        int         `json:"book"`
	Part        string      `json:"part"`
	ChapterFrom int         `json:"chapterFrom"`
	ChapterTo   int         `json:"chapterTo"`
	States      []StateView `json:"states"`
}

// HiddenView says how much is kept back: the chapters left in the reader's book and the main-line books after it.
type HiddenView struct {
	ChapterFrom int `json:"chapterFrom"`
	ChapterTo   int `json:"chapterTo"`
	LaterBooks  int `json:"laterBooks"`
}

type RelationItem struct {
	Slug string `json:"slug,omitempty"` // set when the other side has a page the reader may open
	Name string `json:"name"`
	Role string `json:"role"`
}

type RelationGroup struct {
	Group string         `json:"group"`
	Items []RelationItem `json:"items"`
}

type RefItem struct {
	Fact     string `json:"fact"`
	Quote    string `json:"quote,omitempty"`
	Sequence *int   `json:"sequence,omitempty"`
}

type ReferenceView struct {
	N       int       `json:"n"`
	Book    int       `json:"book"`
	Chapter int       `json:"chapter"`
	Label   string    `json:"label"`
	Items   []RefItem `json:"items"`
}

type SuggestionView struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Entity    string `json:"entity"`
	Other     string `json:"other,omitempty"`
	OtherName string `json:"otherName,omitempty"`
	Evidence  string `json:"evidence"`
}

type BookPresence struct {
	Book     int    `json:"book"`
	Title    string `json:"title"`
	Side     bool   `json:"side"`
	Chapters int    `json:"chapters"`
}

type PageView struct {
	Slug        string           `json:"slug"`
	Name        string           `json:"name"`
	Kind        string           `json:"kind"`
	Page        bool             `json:"page"`
	Aliases     []string         `json:"aliases"`
	Books       []BookPresence   `json:"books"`
	Situation   *StateView       `json:"situation"`
	Who         *WhoView         `json:"who"`
	Biography   []BioBook        `json:"biography"`
	InProgress  *ProgressView    `json:"inProgress"`
	Hidden      *HiddenView      `json:"hidden"`
	Personality []SentenceView   `json:"personality"`
	Relations   []RelationGroup  `json:"relations"`
	Timeline    []StateView      `json:"timeline"`
	References  []ReferenceView  `json:"references"`
	Suggestions []SuggestionView `json:"suggestions,omitempty"`
}

type sentenceRow struct {
	id        int64
	section   int64
	paragraph int
	position  int
	text      string
	refs      []refRow
}

type refRow struct {
	book, chapter int
	fact          string
	quote         sql.NullString
	sequence      sql.NullInt64
}

type sectionRow struct {
	id             int64
	kind           string
	book, from, to sql.NullInt64
	part           string
	position       int
	sentences      []*sentenceRow
}

// Page builds what the reader sees of one entity. staff adds the open identity suggestions (DEC-181).
func (p *Project) Page(ctx context.Context, db *sql.DB, slug string, c Cursor, staff bool) (*PageView, error) {
	var e entityRow
	err := db.QueryRowContext(ctx, `SELECT id, slug, name, kind, page FROM wiki_entities WHERE project_id = $1 AND slug = $2`, p.ID, slug).
		Scan(&e.id, &e.slug, &e.name, &e.kind, &e.page)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	counts, err := p.visibleCounts(ctx, db, c)
	if err != nil {
		return nil, err
	}
	if counts[e.id] == 0 {
		return nil, ErrNotFound // not yet in the story for this reader: not even known to exist
	}
	v := &PageView{Slug: e.slug, Name: e.name, Kind: e.kind, Page: e.page, Aliases: []string{}, Books: []BookPresence{},
		Biography: []BioBook{}, Personality: []SentenceView{}, Relations: []RelationGroup{}, Timeline: []StateView{}, References: []ReferenceView{}}

	aliases, err := p.visibleAliases(ctx, db, c, e.id)
	if err != nil {
		return nil, err
	}
	if a := aliases[e.id]; a != nil {
		v.Aliases = a
	}
	// Presence by book.
	rows, err := db.QueryContext(ctx, `SELECT book, chapter FROM wiki_appearances WHERE entity_id = $1 ORDER BY book, chapter`, e.id)
	if err != nil {
		return nil, err
	}
	per := map[int]int{}
	for rows.Next() {
		var b, ch int
		if err := rows.Scan(&b, &ch); err != nil {
			rows.Close()
			return nil, err
		}
		if p.Sees(c, b, ch) {
			per[b]++
		}
	}
	rows.Close()
	for i, b := range p.Books {
		if per[i] > 0 {
			v.Books = append(v.Books, BookPresence{Book: i, Title: b.Title, Side: b.Side, Chapters: per[i]})
		}
	}

	// States: the timeline, the situation and the part in progress.
	rows, err = db.QueryContext(ctx, `SELECT book, chapter, text FROM wiki_states WHERE entity_id = $1 ORDER BY book, chapter`, e.id)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var s StateView
		if err := rows.Scan(&s.Book, &s.Chapter, &s.Text); err != nil {
			rows.Close()
			return nil, err
		}
		if !p.Sees(c, s.Book, s.Chapter) {
			continue
		}
		s.Label = p.Label(s.Book, s.Chapter)
		v.Timeline = append(v.Timeline, s)
		if !p.side(s.Book) {
			st := s
			v.Situation = &st
		}
	}
	rows.Close()
	v.InProgress, v.Hidden = p.progress(c, v.Timeline)

	// Sections and sentences.
	sections, err := loadSections(ctx, db, e.id)
	if err != nil {
		return nil, err
	}
	refNumber := map[Point]int{}
	refs := map[Point]*ReferenceView{}
	number := func(s *sentenceRow) SentenceView {
		sv := SentenceView{ID: s.id, Text: s.text, Refs: []int{}}
		for _, r := range s.refs {
			pt := Point{r.book, r.chapter}
			n, ok := refNumber[pt]
			if !ok {
				n = len(refNumber) + 1
				refNumber[pt] = n
				refs[pt] = &ReferenceView{N: n, Book: r.book, Chapter: r.chapter, Label: p.Label(r.book, r.chapter), Items: []RefItem{}}
			}
			rv := refs[pt]
			item := RefItem{Fact: r.fact}
			if r.quote.Valid {
				item.Quote = r.quote.String
			}
			if r.sequence.Valid {
				seq := int(r.sequence.Int64)
				item.Sequence = &seq
			}
			dup := false
			for _, x := range rv.Items {
				if x.Fact == item.Fact && x.Quote == item.Quote {
					dup = true
					break
				}
			}
			if !dup && len(rv.Items) < 8 {
				rv.Items = append(rv.Items, item)
			}
			if len(sv.Refs) == 0 || sv.Refs[len(sv.Refs)-1] != n {
				seen := false
				for _, x := range sv.Refs {
					if x == n {
						seen = true
					}
				}
				if !seen {
					sv.Refs = append(sv.Refs, n)
				}
			}
		}
		return sv
	}
	// A sentence is shown only when every reference it rests on is reached: a section is placed at the end of its part, but the
	// generator may have leaned on a fact from later (the lab chose identity facts from the whole book), and the cursor is the rule.
	reached := func(s *sentenceRow) bool {
		for _, r := range s.refs {
			if !p.Sees(c, r.book, r.chapter) {
				return false
			}
		}
		return len(s.refs) > 0
	}
	paragraphs := func(list []*sentenceRow) []Paragraph {
		var out []Paragraph
		last := -1
		for _, s := range list {
			if !reached(s) {
				continue
			}
			if s.paragraph != last || len(out) == 0 {
				out = append(out, Paragraph{})
				last = s.paragraph
			}
			out[len(out)-1] = append(out[len(out)-1], number(s))
		}
		if out == nil {
			out = []Paragraph{}
		}
		return out
	}
	// "Quem é": the last one the cursor reached (main line only).
	var who *sectionRow
	for _, s := range sections {
		if s.kind != "who" || !s.book.Valid || p.side(int(s.book.Int64)) || !p.Sees(c, int(s.book.Int64), int(s.to.Int64)) {
			continue
		}
		if who == nil || s.book.Int64 > who.book.Int64 || (s.book.Int64 == who.book.Int64 && s.to.Int64 > who.to.Int64) {
			who = s
		}
	}
	if who != nil {
		if ps := paragraphs(who.sentences); len(ps) > 0 {
			v.Who = &WhoView{Book: int(who.book.Int64), Part: who.part, Label: p.Label(int(who.book.Int64), int(who.to.Int64)), Paragraphs: ps}
		}
	}
	// Biography: the parts read, by book (main books first, side books after).
	byBook := map[int]*BioBook{}
	for _, s := range sections {
		if s.kind != "bio" || !s.book.Valid || len(s.sentences) == 0 || !p.Sees(c, int(s.book.Int64), int(s.to.Int64)) {
			continue
		}
		b := int(s.book.Int64)
		ps := paragraphs(s.sentences)
		if len(ps) == 0 {
			continue
		}
		bb := byBook[b]
		if bb == nil {
			bb = &BioBook{Book: b, Title: p.Books[b].Title, Side: p.Books[b].Side, Parts: []BioPart{}}
			byBook[b] = bb
		}
		bb.Parts = append(bb.Parts, BioPart{Part: s.part, ChapterFrom: int(s.from.Int64), ChapterTo: int(s.to.Int64), Paragraphs: ps})
	}
	var keys []int
	for b := range byBook {
		keys = append(keys, b)
	}
	sort.Slice(keys, func(i, j int) bool {
		si, sj := p.side(keys[i]), p.side(keys[j])
		if si != sj {
			return !si
		}
		return keys[i] < keys[j]
	})
	for _, b := range keys {
		bb := byBook[b]
		sort.Slice(bb.Parts, func(i, j int) bool { return bb.Parts[i].ChapterFrom < bb.Parts[j].ChapterFrom })
		v.Biography = append(v.Biography, *bb)
	}
	// Personality: each sentence once every reference is reached.
	for _, s := range sections {
		if s.kind != "personality" {
			continue
		}
		for _, st := range s.sentences {
			if reached(st) {
				v.Personality = append(v.Personality, number(st))
			}
		}
	}
	for _, pt := range sortedPoints(refs) {
		v.References = append(v.References, *refs[pt])
	}
	sort.Slice(v.References, func(i, j int) bool { return v.References[i].N < v.References[j].N })

	// Relations.
	rows, err = db.QueryContext(ctx, `
		SELECT r.grp, r.other_name, r.role, r.book, r.chapter, o.id, COALESCE(o.slug, ''), COALESCE(o.page, false)
		FROM wiki_relations r LEFT JOIN wiki_entities o ON o.id = r.other_id
		WHERE r.entity_id = $1 ORDER BY r.position`, e.id)
	if err != nil {
		return nil, err
	}
	groups := map[string]*RelationGroup{}
	for rows.Next() {
		var grp, name, role, oslug string
		var book, chapter int
		var oid sql.NullInt64
		var opage bool
		if err := rows.Scan(&grp, &name, &role, &book, &chapter, &oid, &oslug, &opage); err != nil {
			rows.Close()
			return nil, err
		}
		if !p.Sees(c, book, chapter) {
			continue
		}
		item := RelationItem{Name: name, Role: role}
		if oid.Valid && opage && counts[oid.Int64] > 0 {
			item.Slug = oslug
		}
		g := groups[grp]
		if g == nil {
			g = &RelationGroup{Group: grp, Items: []RelationItem{}}
			groups[grp] = g
		}
		g.Items = append(g.Items, item)
	}
	rows.Close()
	for _, grp := range Groups {
		if g := groups[grp]; g != nil {
			v.Relations = append(v.Relations, *g)
		}
	}

	if staff {
		sv, err := p.suggestions(ctx, db, e.slug, c)
		if err != nil {
			return nil, err
		}
		v.Suggestions = sv
	}
	return v, nil
}

func sortedPoints(m map[Point]*ReferenceView) []Point {
	out := make([]Point, 0, len(m))
	for p := range m {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Book != out[j].Book {
			return out[i].Book < out[j].Book
		}
		return out[i].Chapter < out[j].Chapter
	})
	return out
}

func loadSections(ctx context.Context, db *sql.DB, entity int64) ([]*sectionRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT id, kind, book, chapter_from, chapter_to, part, position FROM wiki_sections WHERE entity_id = $1 ORDER BY position`, entity)
	if err != nil {
		return nil, err
	}
	var list []*sectionRow
	byID := map[int64]*sectionRow{}
	for rows.Next() {
		s := &sectionRow{}
		if err := rows.Scan(&s.id, &s.kind, &s.book, &s.from, &s.to, &s.part, &s.position); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, s)
		byID[s.id] = s
	}
	rows.Close()
	if len(list) == 0 {
		return list, nil
	}
	rows, err = db.QueryContext(ctx, `
		SELECT s.id, s.section_id, s.paragraph, s.position, s.text FROM wiki_sentences s JOIN wiki_sections sec ON sec.id = s.section_id
		WHERE sec.entity_id = $1 AND s.hidden_at IS NULL ORDER BY s.section_id, s.position`, entity)
	if err != nil {
		return nil, err
	}
	bySentence := map[int64]*sentenceRow{}
	for rows.Next() {
		st := &sentenceRow{}
		if err := rows.Scan(&st.id, &st.section, &st.paragraph, &st.position, &st.text); err != nil {
			rows.Close()
			return nil, err
		}
		byID[st.section].sentences = append(byID[st.section].sentences, st)
		bySentence[st.id] = st
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `
		SELECT r.sentence_id, r.book, r.chapter, r.fact, r.quote, r.sequence FROM wiki_references r
		JOIN wiki_sentences s ON s.id = r.sentence_id JOIN wiki_sections sec ON sec.id = s.section_id
		WHERE sec.entity_id = $1 ORDER BY r.id`, entity)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var r refRow
		if err := rows.Scan(&id, &r.book, &r.chapter, &r.fact, &r.quote, &r.sequence); err != nil {
			return nil, err
		}
		if st := bySentence[id]; st != nil {
			st.refs = append(st.refs, r)
		}
	}
	return list, rows.Err()
}

// progress finds the part the cursor is inside (not at its end) and the chapters kept back.
func (p *Project) progress(c Cursor, timeline []StateView) (*ProgressView, *HiddenView) {
	if c.Book < 0 || c.Book >= len(p.Books) || p.Books[c.Book].Side {
		return nil, nil
	}
	chapters := p.Books[c.Book].Chapters
	var hidden *HiddenView
	last := LastStoryChapter(chapters)
	later := 0
	for i := c.Book + 1; i < len(p.Books); i++ {
		if !p.Books[i].Side {
			later++
		}
	}
	if c.Chapter < last || later > 0 {
		hidden = &HiddenView{LaterBooks: later}
		if c.Chapter < last {
			hidden.ChapterFrom, hidden.ChapterTo = c.Chapter+1, last
		}
	}
	// The part of the cursor's chapter: the run of chapters with the same part name around it.
	idx := -1
	for i, ch := range chapters {
		if ch.Chapter == c.Chapter {
			idx = i
		}
	}
	if idx < 0 {
		return nil, hidden
	}
	part := chapters[idx].Part
	from, to := idx, idx
	for from > 0 && chapters[from-1].Part == part && !chapters[from-1].Editorial {
		from--
	}
	for to+1 < len(chapters) && chapters[to+1].Part == part && !chapters[to+1].Editorial {
		to++
	}
	if chapters[to].Chapter <= c.Chapter {
		return nil, hidden // the part is read whole: its biography shows
	}
	pv := &ProgressView{Book: c.Book, Part: part, ChapterFrom: chapters[from].Chapter, ChapterTo: chapters[to].Chapter, States: []StateView{}}
	for _, s := range timeline {
		if s.Book == c.Book && s.Chapter >= pv.ChapterFrom && s.Chapter <= c.Chapter {
			pv.States = append(pv.States, s)
		}
	}
	return pv, hidden
}

func (p *Project) suggestions(ctx context.Context, db *sql.DB, slug string, c Cursor) ([]SuggestionView, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT s.id, s.kind, s.entity_slug, COALESCE(s.other_slug, ''), COALESCE(o.name, ''), s.evidence, s.book, s.chapter
		FROM wiki_suggestions s LEFT JOIN wiki_entities o ON o.project_id = s.project_id AND o.slug = s.other_slug
		WHERE s.project_id = $1 AND s.state = 'open' AND (s.entity_slug = $2 OR s.other_slug = $2) ORDER BY s.id`, p.ID, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SuggestionView{}
	for rows.Next() {
		var s SuggestionView
		var book, chapter int
		if err := rows.Scan(&s.ID, &s.Kind, &s.Entity, &s.Other, &s.OtherName, &s.Evidence, &book, &chapter); err != nil {
			return nil, err
		}
		if p.Sees(c, book, chapter) {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}
