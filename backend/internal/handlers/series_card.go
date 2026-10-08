package handlers

import (
	"database/sql"
	"log"

	"github.com/lib/pq"
)

// SeriesCard is what a work stands for when the grid of the library shows its whole series as one card (#187, DEC-134): the
// official collection it is in, how many works of each unit the collection has, and where to go on. The work that carries it is
// the newest of the series, which is what puts the series where its newest work would be.
type SeriesCard struct {
	CollectionID int64  `json:"collectionId"`
	Name         string `json:"name"`
	Volumes      int    `json:"volumes"`
	Chapters     int    `json:"chapters"`
	OneShots     int    `json:"oneShots"`
	CoverURL     string `json:"coverUrl"`
	// NewCount: the works of the series that came in the last days and that the caller has not finished; the same for everybody who has
	// not read them, whether or not they have read the series.
	NewCount int `json:"newCount"`
	// Continue: where the caller goes on, as on the page of the collection; nil when they have read it all.
	Continue *SeriesStep `json:"continue"`
}

// seriesLateral says, for a work `w` of a list, which series it stands in (`sc.collection_id` and its `sc.name`, null when none): the official
// collection that is not retired, when the work has a unit and the collection has at least two works with one (a manga with a
// single volume in the library is a work, and opens its sheet).
const seriesLateral = `
	LEFT JOIN LATERAL (
		SELECT cw.collection_id, c.name
		FROM collection_works cw
		JOIN collections c ON c.id = cw.collection_id AND c.kind = 'official' AND c.retired_at IS NULL
		WHERE cw.work_id = w.id AND cw.official AND w.unit IS NOT NULL
		  AND (SELECT COUNT(*) FROM collection_works x JOIN works wx ON wx.id = x.work_id AND wx.retired_at IS NULL AND wx.unit IS NOT NULL
		       WHERE x.collection_id = cw.collection_id AND x.official) >= 2
	) sc ON TRUE`

// seriesRepresentative is the condition that keeps, of the works of a series that the list would show, only the newest: the
// others stand behind it. The works that count are those that are on the shelf asked for (formatGroup), the only filter a
// list of series has.
func seriesRepresentative(formatGroup string) string {
	shelf := ""
	if cond := shelfConditionOf("w2", formatGroup, "wp2.file_format"); cond != "" {
		shelf = " AND " + cond
	}
	return `(sc.collection_id IS NULL OR w.id = (
		SELECT MAX(w2.id)
		FROM collection_works cw2
		JOIN works w2 ON w2.id = cw2.work_id AND w2.retired_at IS NULL AND w2.unit IS NOT NULL
		LEFT JOIN work_primary wp2 ON wp2.work_id = w2.id
		WHERE cw2.collection_id = sc.collection_id AND cw2.official` + shelf + `))`
}

// hasSeriesWorks says whether the library has any work with a unit: if it has none, the list has nothing to put together and
// the query of the plain list is the one that runs.
func hasSeriesWorks(db *sql.DB) bool {
	var any bool
	if err := db.QueryRow(`SELECT EXISTS (SELECT 1 FROM works WHERE unit IS NOT NULL AND retired_at IS NULL)`).Scan(&any); err != nil {
		log.Println("Error looking for works with a unit:", err)
		return false
	}
	return any
}

// loadSeriesCards reads the card of each series given, for the caller: the name, the works of each unit, the cover of the
// first work that has one (as the collections show), and where to go on.
func loadSeriesCards(db *sql.DB, collectionIDs []int64, userID string) (map[int64]*SeriesCard, error) {
	cards := map[int64]*SeriesCard{}
	if len(collectionIDs) == 0 {
		return cards, nil
	}
	rows, err := db.Query(`
		SELECT cw.collection_id, c.name,
		       COUNT(*) FILTER (WHERE w.unit = 'volume'), COUNT(*) FILTER (WHERE w.unit = 'chapter'), COUNT(*) FILTER (WHERE w.unit = 'oneshot'),
		       COALESCE((array_agg(wp.cover_url ORDER BY cw.position NULLS LAST, w.original_title, w.id)
		                 FILTER (WHERE wp.cover_url IS NOT NULL AND wp.cover_url <> ''))[1], '')
		FROM collection_works cw
		JOIN collections c ON c.id = cw.collection_id
		JOIN works w ON w.id = cw.work_id AND w.retired_at IS NULL AND w.unit IS NOT NULL
		LEFT JOIN work_primary wp ON wp.work_id = w.id
		WHERE cw.collection_id = ANY($1) AND cw.official
		GROUP BY cw.collection_id, c.name`, pq.Array(collectionIDs))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c SeriesCard
		if err := rows.Scan(&c.CollectionID, &c.Name, &c.Volumes, &c.Chapters, &c.OneShots, &c.CoverURL); err != nil {
			return nil, err
		}
		cards[c.CollectionID] = &c
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for id, card := range cards {
		entries, err := loadSeries(db, id, userID)
		if err != nil {
			return nil, err
		}
		card.Continue = continueSeries(entries)
		for _, e := range entries {
			if e.New && !e.Done {
				card.NewCount++
			}
		}
	}
	return cards, nil
}
