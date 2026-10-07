package handlers

import (
	"fmt"
	"testing"
)

func favRow(id int, series string, index float64) favoriteRow {
	author := fmt.Sprintf("author %d", id)
	return favoriteRow{item: FavoriteSeriesItem{WorkID: id, Title: "t", Author: author, CoverURL: fmt.Sprintf("cover %d", id), SeriesLabel: series}, series: series, index: index}
}

func TestGroupFavoriteSeries(t *testing.T) {
	got := groupFavoriteSeries([]favoriteRow{
		favRow(2, "", 0),
		favRow(1, "Duna", 3),
		favRow(4, "Duna", 1),
		favRow(3, "Duna", 1), // same number as the one before: the lowest id leads
		favRow(6, "Duna", 2), // after the lead has changed, it is the new lead that the next one is measured against
		favRow(5, "Outra", 0),
	})
	if len(got) != 3 {
		t.Fatalf("items = %+v", got)
	}
	if g := got[0]; g.Kind != "work" || g.WorkID != 2 || g.FavoriteCount != 1 {
		t.Errorf("loose item = %+v", g)
	}
	if g := got[1]; g.Kind != "series" || g.Title != "Duna" || g.WorkID != 3 || g.FavoriteCount != 4 || g.CoverURL != "cover 3" || g.Author != "author 3" {
		t.Errorf("series item = %+v", g)
	}
	if g := got[2]; g.Kind != "series" || g.Title != "Outra" || g.FavoriteCount != 1 {
		t.Errorf("a series of one favorite is still a series item = %+v", g)
	}
	if got := groupFavoriteSeries(nil); got == nil || len(got) != 0 {
		t.Errorf("no favorites must be an empty list, not null: %#v", got)
	}
}
