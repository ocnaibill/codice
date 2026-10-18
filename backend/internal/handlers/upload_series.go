package handlers

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

// seriesPlace is where an upload says its work belongs (DEC-200): the series, the number in it, what unit it is, whether it is a comic or a manga,
// and the title. The bulk upload of a series sends it with each file, so that nobody types it again for every chapter. What is sent is written
// as a person writes it, and locked, so the analysis of the file does not undo it.
type seriesPlace struct {
	Series    string
	Number    float64 // 0: no number
	Unit      string
	ComicKind string
	Title     string
}

// errInSeries says the number is already taken in the series (DEC-200): the file is not stored, and the answer names the work that holds it.
type errInSeries struct {
	WorkID int
	Title  string
}

func (e *errInSeries) Error() string { return "the series already has this number" }

const maxUploadFieldBytes = 4 << 10

// readPlaceField reads the value of a small text field of the upload form.
func readPlaceField(part io.Reader) (string, bool) {
	b, err := io.ReadAll(io.LimitReader(part, maxUploadFieldBytes+1))
	if err != nil || len(b) > maxUploadFieldBytes {
		return "", false
	}
	return strings.TrimSpace(string(b)), true
}

// parsePlace builds the place from the fields of the form that came before the file. nil when none was sent; false when what was sent is not
// valid. A number, a unit or a kind without a series is allowed: the series is what places a work in a collection, the rest only describes it.
func parsePlace(fields map[string]string) (*seriesPlace, bool) {
	if len(fields) == 0 {
		return nil, true
	}
	p := &seriesPlace{
		Series:    strings.Join(strings.Fields(fields["series"]), " "),
		Unit:      fields["unit"],
		ComicKind: fields["comic_kind"],
		Title:     strings.Join(strings.Fields(fields["title"]), " "),
	}
	if !validUnit(p.Unit) || !validComicKind(p.ComicKind) {
		return nil, false
	}
	if utf8.RuneCountInString(p.Series) > maxCollectionName || utf8.RuneCountInString(p.Title) > 512 {
		return nil, false
	}
	if raw := fields["series_index"]; raw != "" {
		n, ok := parseNumber(raw)
		if !ok || p.Series == "" {
			return nil, false
		}
		p.Number = n
	}
	if *p == (seriesPlace{}) {
		return nil, true
	}
	return p, true
}

func parseNumber(s string) (float64, bool) {
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	var whole, frac, seenDot bool
	// strconv would accept "1e3", "Inf" and "0x1p-2"; a number of a series is digits with at most one decimal point.
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
			if seenDot {
				frac = true
			} else {
				whole = true
			}
		case c == '.' && !seenDot:
			seenDot = true
		default:
			return 0, false
		}
	}
	if !whole || (seenDot && !frac) {
		return 0, false
	}
	n, err := strconv.ParseFloat(s, 64)
	if err != nil || n < 0 || n > 100000 || math.IsNaN(n) {
		return 0, false
	}
	return n, true
}

// holderOf is the work that already holds the number of the series in this unit, 0 if none does. The series is found by the texts that lead
// to its collection, so a series that was renamed is still found by its old name.
func holderOf(ctx context.Context, q interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}, p *seriesPlace, except int) (*errInSeries, error) {
	if p == nil || p.Series == "" || p.Number <= 0 {
		return nil, nil
	}
	var h errInSeries
	err := q.QueryRowContext(ctx, `
		SELECT w.id, w.original_title
		FROM collection_keys k
		JOIN collection_works cw ON cw.collection_id = k.collection_id AND cw.official
		JOIN works w ON w.id = cw.work_id
		WHERE k.key = series_key($1) AND cw.position = $2::real AND COALESCE(w.unit, '') = $3
		  AND w.retired_at IS NULL AND w.id <> $4
		ORDER BY w.id LIMIT 1`, p.Series, p.Number, p.Unit, except).Scan(&h.WorkID, &h.Title)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// placeInSeries writes the place on a work that was just made, in the transaction that makes it. A second chapter with the same number that
// arrives at the same moment waits for the first (a lock on the series), so two uploads cannot both take the number.
func placeInSeries(ctx context.Context, tx *sql.Tx, workID int, actor string, p *seriesPlace) error {
	if p.Series != "" {
		if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('series-upload:' || series_key($1), 0))`, p.Series); err != nil {
			return err
		}
		if held, err := holderOf(ctx, tx, p, workID); err != nil || held != nil {
			if err != nil {
				return err
			}
			return held
		}
	}
	cur, retired, err := readWorkFields(tx, workID)
	if err != nil {
		return err
	}
	next := cur
	locks := map[string]*bool{}
	yes := true
	if p.Series != "" {
		next.Series, next.SeriesIndex = p.Series, p.Number
		locks["series"] = &yes
	}
	if p.Title != "" {
		next.Title = p.Title
		locks["title"] = &yes
	}
	next.Unit, next.ComicKind = p.Unit, p.ComicKind
	if _, err := applyWorkFields(ctx, tx, workID, actor, sourceManual, cur, retired, next, locks); err != nil {
		return err
	}
	// A manga is read from right to left: the first chapter of a new series says so, and a direction a person already chose is left as it is.
	if p.Series != "" && p.ComicKind == "manga" {
		if _, err := tx.ExecContext(ctx, `
			UPDATE collections SET reading_direction = 'rtl'
			WHERE id IN (SELECT collection_id FROM collection_keys WHERE key = series_key($1))
			  AND kind = 'official' AND reading_direction IS NULL AND retired_at IS NULL`, p.Series); err != nil {
			return err
		}
	}
	return nil
}
