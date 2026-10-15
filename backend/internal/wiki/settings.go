package wiki

import (
	"context"
	"database/sql"
	"encoding/json"
)

// The settings of the generation, apart from the provider (DEC-186): who gets a full page.

// SettingGeneration is the key of the settings of the generator.
const SettingGeneration = "wiki.settings"

// DefaultMinChapters is the least chapters an entity must appear in (the books of the series added up) to have a full page.
const DefaultMinChapters = 5

// MaxMinChapters is the biggest cut that can be set: past it nobody would have a page.
const MaxMinChapters = 200

// Settings is what the owner chooses for the generation.
type Settings struct {
	MinChapters int `json:"minChapters"`
}

// LoadSettings reads them, with the default for what was never set.
func LoadSettings(ctx context.Context, db *sql.DB) (Settings, error) {
	s := Settings{MinChapters: DefaultMinChapters}
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, SettingGeneration).Scan(&raw)
	if err == sql.ErrNoRows {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	var stored Settings
	if json.Unmarshal(raw, &stored) == nil && stored.MinChapters >= 1 && stored.MinChapters <= MaxMinChapters {
		s.MinChapters = stored.MinChapters
	}
	return s, nil
}

// SaveSettings stores them.
func SaveSettings(ctx context.Context, db *sql.DB, s Settings, userID string) error {
	value, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, `INSERT INTO settings (key, value, updated_by) VALUES ($1, $2::jsonb, NULLIF($3, '')::uuid)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		SettingGeneration, value, userID)
	return err
}

// PreviewItem says, for one wiki that exists, how many of its entities would have a full page with a cut.
type PreviewItem struct {
	ProjectID int64          `json:"projectId"`
	Title     string         `json:"title"`
	Entities  int            `json:"entities"`
	Pages     int            `json:"pages"`
	Would     int            `json:"would"`
	ByKind    map[string]int `json:"byKind"`
}

// Preview counts, in each wiki, the entities seen in at least `min` chapters, by kind. It looks at what was generated: a wiki that
// does not exist yet has nothing to count.
func Preview(ctx context.Context, db *sql.DB, min int) ([]PreviewItem, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT p.id, COALESCE(c.name, w.original_title, ''), e.kind, e.page, e.chapters
		FROM wiki_projects p
		LEFT JOIN collections c ON c.id = p.collection_id
		LEFT JOIN works w ON w.id = p.work_id
		LEFT JOIN LATERAL (
			SELECT e.kind, e.page, (SELECT count(DISTINCT (a.book, a.chapter)) FROM wiki_appearances a WHERE a.entity_id = e.id) AS chapters
			FROM wiki_entities e WHERE e.project_id = p.id) e ON TRUE
		ORDER BY lower(COALESCE(c.name, w.original_title, '')), p.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PreviewItem{}
	byID := map[int64]*PreviewItem{}
	for rows.Next() {
		var id int64
		var title string
		var kind sql.NullString
		var page sql.NullBool
		var chapters sql.NullInt64
		if err := rows.Scan(&id, &title, &kind, &page, &chapters); err != nil {
			return nil, err
		}
		it := byID[id]
		if it == nil {
			out = append(out, PreviewItem{ProjectID: id, Title: title, ByKind: map[string]int{}})
			it = &out[len(out)-1]
			byID[id] = it
		}
		if !kind.Valid {
			continue
		}
		it.Entities++
		if page.Bool {
			it.Pages++
		}
		if int(chapters.Int64) >= min {
			it.Would++
			it.ByKind[kind.String]++
		}
	}
	return out, rows.Err()
}
