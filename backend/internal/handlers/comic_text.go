package handlers

import (
	"context"
	"database/sql"
)

// queueComicText asks for the text of the comics that were read by OCR to be made again (DEC-176). What was read does not depend on the
// direction a comic is read in, but the order of the balloons of a page, in the text the search sees, does: when the direction of a series
// (or the kind of a work, which decides it when nothing else does) changes, the text is made again from the pages already read, which
// costs seconds and reads no picture. Only the works that have a page with speech are queued, and asking twice while one is waiting is one
// request. `filter` is a condition on the editions `e` of the works, with `arg` as $1.
func queueComicText(ctx context.Context, tx *sql.Tx, filter string, arg any) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO jobs (type, work_id, payload, priority)
		SELECT DISTINCT 'extract_text', e.work_id, '{"force": true}'::jsonb, 0
		FROM editions e
		JOIN files f ON f.edition_id = e.id
		JOIN works w ON w.id = e.work_id AND w.retired_at IS NULL
		WHERE `+filter+` AND EXISTS (SELECT 1 FROM comic_ocr_pages p WHERE p.file_id = f.id AND p.state = 'done')
		ON CONFLICT (type, work_id) WHERE state IN ('pending', 'running') DO NOTHING`, arg)
	return err
}
