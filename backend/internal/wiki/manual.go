package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// What the staff writes by hand (DEC-186): an entity of the graph, a section of a page, a tie, the state of an entity at a chapter, an
// edge of the graph, an event with its participants. Each has the point (book, chapter) from which it may be shown, so the cut of
// DEC-179 holds for it too, and each is kept for the work or series: a new generation puts it back (applyManual).

// Kinds of item.
const (
	ItemEntity   = "entity"
	ItemSection  = "section"
	ItemRelation = "relation"
	ItemState    = "state"
	ItemEdge     = "edge"
)

const (
	maxItemsPerScope = 5000
	maxParagraphs    = 20
	maxParagraphText = 4000
	maxStateText     = 1000
)

// ManualInput is one thing the staff writes.
type ManualInput struct {
	Kind      string         `json:"kind"`
	Entity    string         `json:"entity"`    // the slug of the entity it is of (not for kind "entity")
	Name      string         `json:"name"`      // kind "entity": the name of the new one
	Other     string         `json:"other"`     // the slug of the other entity of a tie or an edge
	OtherName string         `json:"otherName"` // a tie to something that is not in the wiki
	Book      int            `json:"book"`
	Chapter   int            `json:"chapter"`
	Data      map[string]any `json:"data"`
}

// ManualItem is what is kept.
type ManualItem struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	Entity    string          `json:"entity"`
	Other     string          `json:"other,omitempty"`
	Data      json.RawMessage `json:"data"`
	Book      int             `json:"book"`
	Chapter   int             `json:"chapter"`
	Author    string          `json:"author"`
	CreatedAt time.Time       `json:"createdAt"`
}

type itemRow struct {
	id                    int64
	kind                  string
	entityKey, entityName string
	otherKey, otherName   string
	data                  map[string]any
	book, chapter         int
	author                string
}

func str(m map[string]any, k string) string {
	v, _ := m[k].(string)
	return strings.TrimSpace(v)
}

func okGroup(g string) bool { return in(Groups, g) }

// paragraphsOf is the paragraphs of a section as they were typed: a list, or text with blank lines between them.
func paragraphsOf(data map[string]any) []string {
	var out []string
	add := func(text string) {
		for _, p := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n\n") {
			if t := strings.TrimSpace(p); t != "" {
				out = append(out, t)
			}
		}
	}
	switch v := data["paragraphs"].(type) {
	case []string:
		for _, p := range v {
			add(p)
		}
	case []any:
		for _, p := range v {
			if t, ok := p.(string); ok {
				add(t)
			}
		}
	case string:
		add(v)
	}
	return out
}

func manualErr(format string, a ...any) error { return ErrIdentity{Msg: fmt.Sprintf(format, a...)} }

// check says whether an item may be written, and cleans it.
func (m *ManualInput) check(ctx context.Context, tx *sql.Tx, projectID int64) error {
	var chapters int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM wiki_chapters WHERE project_id = $1 AND book = $2 AND chapter = $3`,
		projectID, m.Book, m.Chapter).Scan(&chapters); err != nil {
		return err
	}
	if chapters == 0 {
		return manualErr("O ponto escolhido (livro e capítulo) não existe nesta wiki.")
	}
	if m.Data == nil {
		m.Data = map[string]any{}
	}
	switch m.Kind {
	case ItemEntity:
		m.Name = strings.Join(strings.Fields(m.Name), " ")
		if m.Name == "" || utf8.RuneCountInString(m.Name) > 160 || Key(m.Name) == "" {
			return manualErr("O nome da entidade tem de ter de 1 a 160 caracteres.")
		}
		if !in(Kinds, str(m.Data, "kind")) {
			return manualErr("Escolha o tipo da entidade.")
		}
		var aliases []string
		if list, ok := m.Data["aliases"].([]any); ok {
			for _, a := range list {
				if t, ok := a.(string); ok && Key(t) != "" && Key(t) != Key(m.Name) && len(aliases) < 10 && utf8.RuneCountInString(t) <= 160 {
					aliases = append(aliases, strings.TrimSpace(t))
				}
			}
		}
		m.Data = map[string]any{"kind": str(m.Data, "kind"), "aliases": aliases}
	case ItemSection:
		kind := str(m.Data, "kind")
		if kind != "who" && kind != "bio" && kind != "personality" {
			return manualErr("Escolha a seção: Quem é, Biografia ou Personalidade.")
		}
		paras := paragraphsOf(m.Data)
		if len(paras) == 0 || len(paras) > maxParagraphs {
			return manualErr("Escreva de 1 a %d parágrafos.", maxParagraphs)
		}
		for _, p := range paras {
			if utf8.RuneCountInString(p) > maxParagraphText {
				return manualErr("Cada parágrafo tem até %d caracteres.", maxParagraphText)
			}
		}
		part := str(m.Data, "part")
		if utf8.RuneCountInString(part) > 200 {
			return manualErr("O nome da parte tem até 200 caracteres.")
		}
		m.Data = map[string]any{"kind": kind, "part": part, "paragraphs": paras}
	case ItemRelation:
		if !okGroup(str(m.Data, "group")) {
			return manualErr("Escolha o grupo do vínculo.")
		}
		role := str(m.Data, "role")
		if utf8.RuneCountInString(role) > 200 {
			return manualErr("O papel tem até 200 caracteres.")
		}
		if m.Other == "" && strings.TrimSpace(m.OtherName) == "" {
			return manualErr("Diga com quem ou com o quê é o vínculo.")
		}
		m.Data = map[string]any{"group": str(m.Data, "group"), "role": role}
	case ItemState:
		text := str(m.Data, "text")
		if text == "" || utf8.RuneCountInString(text) > maxStateText {
			return manualErr("A situação tem de 1 a %d caracteres.", maxStateText)
		}
		m.Data = map[string]any{"text": text}
	case ItemEdge:
		if m.Other == "" {
			return manualErr("Escolha a entidade a ligar.")
		}
		label := str(m.Data, "label")
		if label == "" {
			label = "ligado a"
		}
		if utf8.RuneCountInString(label) > 300 {
			return manualErr("O texto do vínculo tem até 300 caracteres.")
		}
		m.Data = map[string]any{"label": label}
	default:
		return manualErr("Tipo de item desconhecido.")
	}
	return nil
}

func slugFor(name string, taken map[string]bool) string {
	base := strings.ReplaceAll(fold(name), " ", "-")
	if base == "" {
		base = "entidade"
	}
	if r := []rune(base); len(r) > 100 {
		base = string(r[:100])
	}
	slug, n := base, 2
	for taken[slug] {
		slug = base + "-" + strconv.Itoa(n)
		n++
	}
	return slug
}

// applyManual puts one item on the project. An item whose entity is not in this generation is left for the next (it stays kept).
func applyManual(ctx context.Context, tx *sql.Tx, projectID int64, it itemRow) error {
	byKey, byID, err := entitiesByKey(ctx, tx, projectID)
	if err != nil {
		return err
	}
	exec := func(q string, args ...any) error {
		_, err := tx.ExecContext(ctx, q, args...)
		return err
	}
	switch it.kind {
	case ItemEntity:
		var aliases []string
		switch list := it.data["aliases"].(type) {
		case []string:
			aliases = list
		case []any: // read back from the database
			for _, a := range list {
				if t, ok := a.(string); ok {
					aliases = append(aliases, t)
				}
			}
		}
		var id int64
		if e, ok := find(byKey, it.entityKey); ok {
			id = e.id // the generation wrote it too: the manual item adds to it
		} else {
			taken := map[string]bool{}
			for _, e := range byID {
				taken[e.slug] = true
			}
			if err := tx.QueryRowContext(ctx, `INSERT INTO wiki_entities (project_id, slug, name, kind, page, manual_item_id) VALUES ($1, $2, $3, $4, false, $5) RETURNING id`,
				projectID, slugFor(it.entityName, taken), it.entityName, str(it.data, "kind"), it.id).Scan(&id); err != nil {
				return err
			}
		}
		if err := exec(`INSERT INTO wiki_appearances (entity_id, book, chapter) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`, id, it.book, it.chapter); err != nil {
			return err
		}
		for _, t := range aliases {
			if Key(t) != "" {
				if err := exec(`INSERT INTO wiki_aliases (entity_id, alias, book, chapter) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING`, id, t, it.book, it.chapter); err != nil {
					return err
				}
			}
		}
		return nil
	}
	e, ok := find(byKey, it.entityKey)
	if !ok {
		return nil
	}
	switch it.kind {
	case ItemSection:
		kind := str(it.data, "kind")
		var book, from, to sql.NullInt64
		if kind != "personality" {
			book = sql.NullInt64{Int64: int64(it.book), Valid: true}
			from = sql.NullInt64{Int64: int64(it.chapter), Valid: true}
			to = from
		}
		var sec int64
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO wiki_sections (entity_id, kind, book, chapter_from, chapter_to, part, position, manual_item_id, manual_by)
			VALUES ($1, $2, $3, $4, $5, $6, 1000, $7, $8) RETURNING id`,
			e.id, kind, book, from, to, str(it.data, "part"), it.id, it.author).Scan(&sec); err != nil {
			return err
		}
		for i, text := range paragraphsOf(it.data) {
			var sid int64
			if err := tx.QueryRowContext(ctx, `INSERT INTO wiki_sentences (section_id, paragraph, position, text) VALUES ($1, $2, $3, $4) RETURNING id`,
				sec, i+1, i, text).Scan(&sid); err != nil {
				return err
			}
			if err := exec(`INSERT INTO wiki_references (sentence_id, book, chapter, fact) VALUES ($1, $2, $3, $4)`,
				sid, it.book, it.chapter, "Escrito à mão por "+it.author); err != nil {
				return err
			}
		}
		if kind != "personality" && !e.page {
			return exec(`UPDATE wiki_entities SET page = true WHERE id = $1`, e.id)
		}
	case ItemRelation:
		var other sql.NullInt64
		if o, ok := find(byKey, it.otherKey); ok && o.id != e.id {
			other = sql.NullInt64{Int64: o.id, Valid: true}
		}
		return exec(`INSERT INTO wiki_relations (entity_id, other_id, other_name, grp, role, book, chapter, position, manual_item_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 1000, $8)`,
			e.id, other, it.otherName, str(it.data, "group"), str(it.data, "role"), it.book, it.chapter, it.id)
	case ItemState:
		return exec(`INSERT INTO wiki_states (entity_id, book, chapter, text, manual_item_id) VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (entity_id, book, chapter) DO UPDATE SET text = EXCLUDED.text, manual_item_id = EXCLUDED.manual_item_id`,
			e.id, it.book, it.chapter, str(it.data, "text"), it.id)
	case ItemEdge:
		o, ok := find(byKey, it.otherKey)
		if !ok || o.id == e.id {
			return nil
		}
		return exec(`INSERT INTO wiki_edges (project_id, a_id, b_id, label, book, chapter, manual_item_id) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			projectID, e.id, o.id, str(it.data, "label"), it.book, it.chapter, it.id)
	}
	return nil
}

// applyManualItems puts everything kept for the work or series on a project that was just written.
func applyManualItems(ctx context.Context, tx *sql.Tx, projectID int64) error {
	scope, err := projectScope(ctx, tx, projectID)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `
		SELECT id, kind, entity_key, entity_name, other_key, other_name, data, book, chapter, author_name FROM wiki_manual_items
		WHERE (work_id IS NOT NULL AND work_id = $1) OR (collection_id IS NOT NULL AND collection_id = $2)
		ORDER BY CASE kind WHEN 'entity' THEN 0 ELSE 1 END, id`, scope.work, scope.collection)
	if err != nil {
		return err
	}
	var items []itemRow
	for rows.Next() {
		var it itemRow
		var raw []byte
		if err := rows.Scan(&it.id, &it.kind, &it.entityKey, &it.entityName, &it.otherKey, &it.otherName, &raw, &it.book, &it.chapter, &it.author); err != nil {
			rows.Close()
			return err
		}
		_ = json.Unmarshal(raw, &it.data)
		items = append(items, it)
	}
	rows.Close()
	for _, it := range items {
		// A point the new generation does not have (the book changed) leaves the item kept, and not shown.
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM wiki_chapters WHERE project_id = $1 AND book = $2 AND chapter = $3`,
			projectID, it.book, it.chapter).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			continue
		}
		if err := applyManual(ctx, tx, projectID, it); err != nil {
			return err
		}
	}
	return nil
}

// CreateManual writes what the staff asked for, all or nothing, in the wiki that exists and kept for the next generations.
func CreateManual(ctx context.Context, db *sql.DB, projectID int64, inputs []ManualInput, userID string) ([]int64, []string, error) {
	if len(inputs) == 0 || len(inputs) > 30 {
		return nil, nil, manualErr("Mande de 1 a 30 itens de uma vez.")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	scope, err := projectScope(ctx, tx, projectID)
	if err != nil {
		return nil, nil, err
	}
	var count int
	if err := tx.QueryRowContext(ctx, `
		SELECT count(*) FROM wiki_manual_items WHERE (work_id IS NOT NULL AND work_id = $1) OR (collection_id IS NOT NULL AND collection_id = $2)`,
		scope.work, scope.collection).Scan(&count); err != nil {
		return nil, nil, err
	}
	if count+len(inputs) > maxItemsPerScope {
		return nil, nil, manualErr("Há itens demais escritos à mão nesta wiki (o limite é %d).", maxItemsPerScope)
	}
	var author string
	_ = tx.QueryRowContext(ctx, `SELECT username FROM users WHERE id = NULLIF($1, '')::uuid`, userID).Scan(&author)
	if author == "" {
		author = "alguém do staff"
	}
	var ids []int64
	slugs := make([]string, 0, len(inputs)) // the slug of the entity an "entity" item made (or found), "" for the others
	for i := range inputs {
		m := &inputs[i]
		if err := m.check(ctx, tx, projectID); err != nil {
			return nil, nil, err
		}
		// "@2" is the entity the item 2 of this same request created: its slug is not known to who is writing.
		for _, ref := range []*string{&m.Entity, &m.Other} {
			if n, err := strconv.Atoi(strings.TrimPrefix(*ref, "@")); strings.HasPrefix(*ref, "@") && err == nil {
				if n < 0 || n >= len(slugs) || slugs[n] == "" {
					return nil, nil, manualErr("O item %d não cria uma entidade antes deste.", n)
				}
				*ref = slugs[n]
			}
		}
		it := itemRow{kind: m.Kind, data: m.Data, book: m.Book, chapter: m.Chapter, author: author}
		// the entity it is of, and the other one, by what they are called now
		name := func(slug string) (string, error) {
			var n string
			err := tx.QueryRowContext(ctx, `SELECT name FROM wiki_entities WHERE project_id = $1 AND slug = $2`, projectID, slug).Scan(&n)
			if errors.Is(err, sql.ErrNoRows) {
				return "", ErrNotFound
			}
			return n, err
		}
		if m.Kind == ItemEntity {
			it.entityName = m.Name
		} else {
			n, err := name(m.Entity)
			if err != nil {
				return nil, nil, err
			}
			it.entityName = n
		}
		it.entityKey = Key(it.entityName)
		switch {
		case m.Other != "":
			n, err := name(m.Other)
			if err != nil {
				return nil, nil, err
			}
			it.otherName = n
		case m.OtherName != "":
			it.otherName = strings.TrimSpace(m.OtherName)
		}
		it.otherKey = Key(it.otherName)
		raw, _ := json.Marshal(m.Data)
		if err := tx.QueryRowContext(ctx, `
			INSERT INTO wiki_manual_items (work_id, collection_id, kind, entity_key, entity_name, other_key, other_name, data, book, chapter, created_by, author_name)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8::jsonb, $9, $10, NULLIF($11, '')::uuid, $12) RETURNING id`,
			scope.work, scope.collection, it.kind, it.entityKey, it.entityName, it.otherKey, it.otherName, raw, it.book, it.chapter, userID, author).Scan(&it.id); err != nil {
			return nil, nil, err
		}
		if err := applyManual(ctx, tx, projectID, it); err != nil {
			return nil, nil, err
		}
		ids = append(ids, it.id)
		slug := ""
		if it.kind == ItemEntity {
			byKey, _, err := entitiesByKey(ctx, tx, projectID)
			if err != nil {
				return nil, nil, err
			}
			if e, ok := find(byKey, it.entityKey); ok {
				slug = e.slug
			}
		}
		slugs = append(slugs, slug)
	}
	if err := audit.Record(ctx, tx, userID, "wiki.manual.create", "wiki", strconv.FormatInt(projectID, 10), map[string]any{"items": len(ids)}); err != nil {
		return nil, nil, err
	}
	return ids, slugs, tx.Commit()
}

// itemOf loads one item of the project's work or series.
func itemOf(ctx context.Context, tx *sql.Tx, projectID, itemID int64) (itemRow, error) {
	var it itemRow
	var raw []byte
	err := tx.QueryRowContext(ctx, `
		SELECT i.id, i.kind, i.entity_key, i.entity_name, i.other_key, i.other_name, i.data, i.book, i.chapter, i.author_name
		FROM wiki_manual_items i JOIN wiki_projects p ON ((i.work_id IS NOT NULL AND i.work_id = p.work_id) OR (i.collection_id IS NOT NULL AND i.collection_id = p.collection_id))
		WHERE p.id = $1 AND i.id = $2 FOR UPDATE OF i`, projectID, itemID).
		Scan(&it.id, &it.kind, &it.entityKey, &it.entityName, &it.otherKey, &it.otherName, &raw, &it.book, &it.chapter, &it.author)
	if errors.Is(err, sql.ErrNoRows) {
		return it, ErrNotFound
	}
	if err != nil {
		return it, err
	}
	_ = json.Unmarshal(raw, &it.data)
	return it, nil
}

// clearRows takes out of the project what an item made.
func clearRows(ctx context.Context, tx *sql.Tx, itemID int64) error {
	for _, q := range []string{
		`DELETE FROM wiki_sections WHERE manual_item_id = $1`,
		`DELETE FROM wiki_relations WHERE manual_item_id = $1`,
		`DELETE FROM wiki_states WHERE manual_item_id = $1`,
		`DELETE FROM wiki_edges WHERE manual_item_id = $1`,
	} {
		if _, err := tx.ExecContext(ctx, q, itemID); err != nil {
			return err
		}
	}
	return nil
}

// UpdateManual rewrites what a section, a tie, a state or an edge says, and the point it comes from.
func UpdateManual(ctx context.Context, db *sql.DB, projectID, itemID int64, input ManualInput, userID string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	it, err := itemOf(ctx, tx, projectID, itemID)
	if err != nil {
		return err
	}
	if it.kind == ItemEntity {
		return manualErr("Uma entidade escrita à mão não se edita: apague e crie de novo.")
	}
	input.Kind, input.Entity = it.kind, ""
	input.Other = ""
	input.OtherName = it.otherName
	if input.Kind == ItemEdge {
		// the other end stays: only the words and the point change
		input.Other = "x"
	}
	if err := input.check(ctx, tx, projectID); err != nil {
		return err
	}
	raw, _ := json.Marshal(input.Data)
	if _, err := tx.ExecContext(ctx, `UPDATE wiki_manual_items SET data = $2::jsonb, book = $3, chapter = $4, updated_at = now() WHERE id = $1`,
		itemID, raw, input.Book, input.Chapter); err != nil {
		return err
	}
	if err := clearRows(ctx, tx, itemID); err != nil {
		return err
	}
	it.data, it.book, it.chapter = input.Data, input.Book, input.Chapter
	if err := applyManual(ctx, tx, projectID, it); err != nil {
		return err
	}
	if err := audit.Record(ctx, tx, userID, "wiki.manual.update", "wiki", strconv.FormatInt(projectID, 10), map[string]any{"item": itemID}); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteManual takes an item away: from the wiki that exists and from the next generations. Taking an entity away takes what was
// written about it with it.
func DeleteManual(ctx context.Context, db *sql.DB, projectID, itemID int64, userID string) (int, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	it, err := itemOf(ctx, tx, projectID, itemID)
	if err != nil {
		return 0, err
	}
	scope, err := projectScope(ctx, tx, projectID)
	if err != nil {
		return 0, err
	}
	removed := 1
	if it.kind == ItemEntity {
		res, err := tx.ExecContext(ctx, `
			DELETE FROM wiki_manual_items WHERE id <> $1 AND ((work_id IS NOT NULL AND work_id = $2) OR (collection_id IS NOT NULL AND collection_id = $3))
			  AND (entity_key = $4 OR other_key = $4)`, itemID, scope.work, scope.collection, it.entityKey)
		if err != nil {
			return 0, err
		}
		n, _ := res.RowsAffected()
		removed += int(n)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM wiki_manual_items WHERE id = $1`, itemID); err != nil {
		return 0, err
	}
	if err := audit.Record(ctx, tx, userID, "wiki.manual.delete", "wiki", strconv.FormatInt(projectID, 10), map[string]any{"item": itemID, "removed": removed}); err != nil {
		return 0, err
	}
	return removed, tx.Commit()
}

// ListManual lists what was written by hand about an entity (its sections, ties, states and edges), for the staff to edit.
func ListManual(ctx context.Context, db *sql.DB, projectID int64, slug string) ([]ManualItem, error) {
	var name string
	var entityID int64
	err := db.QueryRowContext(ctx, `SELECT id, name FROM wiki_entities WHERE project_id = $1 AND slug = $2`, projectID, slug).Scan(&entityID, &name)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	keys := map[string]bool{fold(name): true}
	rows, err := db.QueryContext(ctx, `SELECT alias FROM wiki_aliases WHERE entity_id = $1`, entityID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return nil, err
		}
		keys[fold(a)] = true
	}
	rows.Close()
	rows, err = db.QueryContext(ctx, `
		SELECT i.id, i.kind, i.entity_key, i.other_key, i.other_name, i.data, i.book, i.chapter, i.author_name, i.created_at
		FROM wiki_manual_items i
		JOIN wiki_projects p ON ((i.work_id IS NOT NULL AND i.work_id = p.work_id) OR (i.collection_id IS NOT NULL AND i.collection_id = p.collection_id))
		WHERE p.id = $1 ORDER BY i.created_at, i.id`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ManualItem{}
	for rows.Next() {
		var it ManualItem
		var entityKey, otherKey string
		var raw []byte
		if err := rows.Scan(&it.ID, &it.Kind, &entityKey, &otherKey, &it.Other, &raw, &it.Book, &it.Chapter, &it.Author, &it.CreatedAt); err != nil {
			return nil, err
		}
		if !keys[entityKey] {
			continue
		}
		it.Entity = slug
		it.Data = raw
		out = append(out, it)
	}
	return out, rows.Err()
}
