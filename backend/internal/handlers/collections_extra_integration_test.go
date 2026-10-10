package handlers

import (
	"fmt"
	"testing"
)

func TestExtraUnit_CanBeSaidInTheEditAndInTheClassificationOfTheCollection(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Harry Potter", "volume", "", "")
	if code := s.editWork(ids[1], `,"unit":"extra"`); code != 200 {
		t.Fatalf("edit: %d", code)
	}
	if s.kinds(ids[1]) != "extra|-" {
		t.Errorf("after the edit: %s", s.kinds(ids[1]))
	}
	if code, _ := s.classify(col, `{"unit":"extra","onlyUnset":true}`); code != 200 {
		t.Fatalf("classify: %d", code)
	}
	if s.kinds(ids[0]) != "volume|-" || s.kinds(ids[2]) != "extra|-" {
		t.Errorf("only the work with no unit changed: %s %s", s.kinds(ids[0]), s.kinds(ids[2]))
	}
	// The group has its own order.
	if rec := s.do(admin, "PUT", fmt.Sprintf("/collections/%d/order", col), fmt.Sprintf(`{"workIds":[%d,%d],"unit":"extra"}`, ids[2], ids[1])); rec.Code != 204 {
		t.Errorf("order of the group: %d %s", rec.Code, rec.Body.String())
	}
	// And the message of a refusal names it.
	if code, msg := s.classify(col, `{"unit":"arc"}`); code != 400 || msg != "A unidade é volume, capítulo, único ou complementar." {
		t.Errorf("bad unit: %d %q", code, msg)
	}
}

func TestExtraUnit_IsLeftOutOfTheProgressTheNumbersThatAreMissingAndOfGoingOn(t *testing.T) {
	s := newCatalogStack(t)
	// Volumes 1, 2 and 4 (a gap at 3), and two complementary works, one of them numbered 9.
	col, ids := s.seriesOf("Saga", "volume", "volume", "volume", "extra", "extra")
	s.exec(`UPDATE works SET series_index = 4 WHERE id = $1`, ids[2])
	s.exec(`UPDATE works SET series_index = 9 WHERE id = $1`, ids[4])
	s.read(idAna, ids[0], true)
	s.read(idAna, ids[3], true) // the guide, read to the end
	s.exec(`INSERT INTO work_reading_state (user_id, work_id, finished_at) VALUES ($1, $2, now())`, idAna, ids[4])

	d, code := s.collection(ana, col)
	if code != 200 {
		t.Fatalf("%d", code)
	}
	sum := d.Summary
	if sum.Works != 3 || sum.Finished != 1 || sum.InProgress != 0 {
		t.Errorf("the sequence is the three volumes: %+v", sum)
	}
	// (100 + 0 + 0) / 3
	if sum.Percent != 33.3 {
		t.Errorf("percent = %v", sum.Percent)
	}
	if len(sum.Missing) != 1 || sum.Missing[0].Unit != "volume" || sum.Missing[0].Number != 3 {
		t.Errorf("only the volume 3 is missing, not the numbers of the complementary ones: %+v", sum.Missing)
	}
	// What they are, and what the caller did of them, is said all the same.
	for _, w := range d.Works {
		if w.ID == ids[3] && (w.Unit != "extra" || !w.Completed) {
			t.Errorf("the complementary one: %+v", w)
		}
	}
	// Go on with the next volume of the sequence, and the person has begun it.
	if d.Continue == nil || d.Continue.ID != ids[1] || !d.Continue.Begun {
		t.Errorf("continue: %+v", d.Continue)
	}
}

func TestExtraUnit_ReadingOnlyAComplementaryWorkDoesNotBeginTheSeries(t *testing.T) {
	s := newCatalogStack(t)
	col, ids := s.seriesOf("Saga", "volume", "volume", "extra")
	s.read(idAna, ids[2], true)
	d, _ := s.collection(ana, col)
	if d.Continue == nil || d.Continue.ID != ids[0] || d.Continue.Begun {
		t.Errorf("it is still the beginning of the series: %+v", d.Continue)
	}
	// A series that is only complementary works has no sequence: nothing to go on with, no percent.
	only, oids := s.seriesOf("Só extras", "extra", "extra")
	_ = oids
	e, code := s.collection(ana, only)
	if code != 200 || e.Continue != nil || e.Summary.Works != 0 || e.Summary.Percent != 0 {
		t.Errorf("only extras: %d %+v %+v", code, e.Continue, e.Summary)
	}
}
