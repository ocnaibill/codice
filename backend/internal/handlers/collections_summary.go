package handlers

import (
	"context"
	"database/sql"
	"sort"

	"github.com/lib/pq"
)

// summarize fills, for the works of a collection that can be opened, what the caller has done of each and what it is (the percentage, when
// they finished it, the year, a few lines of the synopsis, the formats, their stars, where they stopped) and says what the whole is: how far
// the caller is, the hours they read, the years it spans, who wrote and translated it, the tags it carries most and the numbers it skips.
// Everything about reading is the caller's own (DEC-017).
func (h *CollectionsHandler) summarize(ctx context.Context, userID, order string, works []CollectionWork) (CollectionSummary, error) {
	sum := CollectionSummary{Missing: []MissingNumber{}, Authors: []PersonRef{}, Translators: []PersonRef{}, Tags: []PersonTag{}}
	var ids []int64
	index := map[int]int{}
	for i := range works {
		works[i].Formats = []string{}
		if works[i].Available && works[i].ID > 0 {
			ids = append(ids, int64(works[i].ID))
			index[works[i].ID] = i
		}
	}
	sum.Works = len(ids)
	if len(ids) == 0 {
		return sum, nil
	}

	rows, err := h.DB.QueryContext(ctx, `
		SELECT w.id, w.original_year, COALESCE(left(w.description, 300), ''),
		       COALESCE((SELECT array_agg(DISTINCT lower(f.format)) FROM files f JOIN editions e ON e.id = f.edition_id
		                 WHERE e.work_id = w.id AND f.availability = 'available' AND COALESCE(f.format, '') <> ''), '{}'),
		       COALESCE(lastrp.percent_complete, 0), (lastrp.completed_at IS NOT NULL), (wrs.work_id IS NOT NULL),
		       lastrp.file_id IS NOT NULL, COALESCE(lastrp.chapter, ''), COALESCE(lastrp.unit_index, 0), COALESCE(lastrp.unit_total, 0),
		       COALESCE(lastrp.format, ''), COALESCE(rt.stars, 0),
		       (SELECT max(rc.completed_at) FROM reading_completions rc WHERE rc.user_id = $1::uuid AND rc.work_id = w.id)
		FROM works w`+lastReadJoin+`
		LEFT JOIN work_ratings rt ON rt.user_id = $1::uuid AND rt.work_id = w.id
		WHERE w.id = ANY($2)`, userID, pq.Array(ids))
	if err != nil {
		return sum, err
	}
	var percents float64
	for rows.Next() {
		var id int
		var year sql.NullInt64
		var synopsis string
		var formats pq.StringArray
		var percent float64
		var done, finished, begun bool
		var chapter, format string
		var unitIndex, unitTotal, stars int
		var completedAt sql.NullTime
		if err := rows.Scan(&id, &year, &synopsis, &formats, &percent, &done, &finished, &begun, &chapter, &unitIndex, &unitTotal, &format, &stars, &completedAt); err != nil {
			rows.Close()
			return sum, err
		}
		cw := &works[index[id]]
		if year.Valid {
			y := int(year.Int64)
			cw.OriginalYear = &y
		}
		cw.Synopsis = synopsis
		cw.Formats = []string(formats)
		if cw.Formats == nil {
			cw.Formats = []string{}
		}
		over := done || finished // the version that counts is finished, or the whole work was marked finished
		cw.Completed = cw.Completed || over
		cw.Started = begun && !over
		switch {
		case over:
			cw.Percent = 100
		case begun:
			cw.Percent = percent
		}
		if completedAt.Valid {
			t := completedAt.Time
			cw.CompletedAt = &t
		}
		cw.Rating = stars
		if cw.Started {
			cw.Chapter, cw.UnitIndex, cw.UnitTotal, cw.ReadFormat = chapter, unitIndex, unitTotal, format
		}
		percents += cw.Percent
		if over {
			sum.Finished++
		} else if begun {
			sum.InProgress++
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return sum, err
	}
	sum.Percent = float64(int(percents/float64(sum.Works)*10+0.5)) / 10

	// The years the works span, the numbers the series skips (only whole numbers, and only from the first to the last).
	numbers := map[string][]float64{}
	for _, cw := range works {
		if !cw.Available {
			continue
		}
		if cw.OriginalYear != nil {
			y := *cw.OriginalYear
			if sum.YearFrom == nil || y < *sum.YearFrom {
				sum.YearFrom = &y
			}
			if sum.YearTo == nil || y > *sum.YearTo {
				sum.YearTo = &y
			}
		}
		if cw.Position != nil && cw.Unit != "oneshot" {
			numbers[cw.Unit] = append(numbers[cw.Unit], *cw.Position)
		}
	}
	units := make([]string, 0, len(numbers))
	for unit := range numbers {
		units = append(units, unit)
	}
	sort.Strings(units) // the volumes after the chapters, and the ones with no unit first: a fixed order
	for _, unit := range units {
		for _, n := range skippedNumbers(numbers[unit]) {
			sum.Missing = append(sum.Missing, MissingNumber{Unit: unit, Number: n})
		}
	}

	if err := h.DB.QueryRowContext(ctx, `
		SELECT COALESCE(sum(r.reading_seconds), 0)
		FROM reading_progress r JOIN files f ON f.id = r.file_id JOIN editions e ON e.id = f.edition_id
		WHERE r.user_id = $1::uuid AND e.work_id = ANY($2)`, userID, pq.Array(ids)).Scan(&sum.ReadingSeconds); err != nil {
		return sum, err
	}

	for _, role := range []struct {
		name string
		into *[]PersonRef
		max  int
	}{{"author", &sum.Authors, 3}, {"translator", &sum.Translators, 2}} {
		prows, err := h.DB.QueryContext(ctx, `
			SELECT p.id, `+displayNameSQL("p", "$3")+`, count(DISTINCT c.work_id) AS n
			FROM work_contributors c JOIN person p ON p.id = c.person_id
			WHERE c.work_id = ANY($1) AND c.role = $2
			GROUP BY p.id ORDER BY n DESC, lower(p.name), p.id LIMIT $4`, pq.Array(ids), role.name, order, role.max)
		if err != nil {
			return sum, err
		}
		for prows.Next() {
			var ref PersonRef
			if err := prows.Scan(&ref.ID, &ref.Name, &ref.Works); err != nil {
				prows.Close()
				return sum, err
			}
			*role.into = append(*role.into, ref)
		}
		prows.Close()
	}

	trows, err := h.DB.QueryContext(ctx, `
		SELECT t.name, count(DISTINCT wt.work_id) AS n
		FROM work_tags wt JOIN tags t ON t.id = wt.tag_id
		WHERE wt.work_id = ANY($1)
		GROUP BY t.name ORDER BY n DESC, lower(t.name), t.name LIMIT $2`, pq.Array(ids), personTags)
	if err != nil {
		return sum, err
	}
	defer trows.Close()
	for trows.Next() {
		var tag PersonTag
		if err := trows.Scan(&tag.Name, &tag.Works); err != nil {
			return sum, err
		}
		sum.Tags = append(sum.Tags, tag)
	}
	return sum, trows.Err()
}

// skippedNumbers are the whole numbers between the first and the last of a series that no work has: 1, 2 and 4 skip 3. A series that has
// half numbers (27,5) or fewer than two numbers says nothing; a number is said once however many works have it.
func skippedNumbers(numbers []float64) []float64 {
	missing := []float64{}
	have := map[int]bool{}
	var whole []int
	for _, n := range numbers {
		if n != float64(int(n)) || n < 1 || n > 10000 {
			continue
		}
		i := int(n)
		if !have[i] {
			have[i] = true
			whole = append(whole, i)
		}
	}
	if len(whole) == 0 {
		return missing
	}
	sort.Ints(whole)
	for n := whole[0]; n < whole[len(whole)-1]; n++ {
		if !have[n] {
			missing = append(missing, float64(n))
		}
	}
	return missing
}
