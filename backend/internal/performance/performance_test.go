package performance_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"testing"

	"github.com/ocnaibill/codice/backend/internal/database"
	"github.com/ocnaibill/codice/backend/internal/performance"
	"github.com/ocnaibill/codice/backend/internal/testdb"
)

var ctx = context.Background()

func intp(n int) *int { return &n }

func TestDefaults_StartAtWhatItHasAlwaysBeen(t *testing.T) {
	d := performance.Defaults(8)
	want := performance.Settings{"catalogReads": 8, "dedupeJobs": 1, "ocrPages": 1, "ocrThreads": 1}
	for k, v := range want {
		if d[k] != v {
			t.Errorf("default %s = %d, want %d", k, d[k], v)
		}
	}
	if performance.Defaults(0)["catalogReads"] != 8 || performance.Defaults(-3)["catalogReads"] != 8 {
		t.Error("a catalog limit of zero or less (the gate is off) must still show the default of 8 on the screen")
	}
	if performance.Defaults(12)["catalogReads"] != 12 {
		t.Error("the catalog default is the one of CODICE_CATALOG_CONCURRENCY")
	}
}

func TestLimits_FollowTheCoresOfTheMachine(t *testing.T) {
	small := performance.Limits(performance.Machine{Cores: 2})
	big := performance.Limits(performance.Machine{Cores: 32})
	none := performance.Limits(performance.Machine{Cores: 0})
	for _, c := range []struct {
		name   string
		limits map[string]performance.Range
		field  string
		max    int
	}{
		{"2 cores", small, "ocrPages", 2}, {"2 cores", small, "ocrThreads", 2}, {"2 cores", small, "dedupeJobs", 2},
		{"32 cores", big, "ocrPages", 8}, {"32 cores", big, "ocrThreads", 8}, {"32 cores", big, "dedupeJobs", 4},
		{"unknown cores", none, "ocrPages", 1},
		{"2 cores", small, "catalogReads", 20}, {"32 cores", big, "catalogReads", 20},
	} {
		if got := c.limits[c.field]; got.Min != 1 || got.Max != c.max {
			t.Errorf("%s %s = %+v, want 1..%d", c.name, c.field, got, c.max)
		}
	}
}

func TestValidate(t *testing.T) {
	m := performance.Machine{Cores: 8}
	ok := []map[string]*int{{"ocrPages": intp(1)}, {"ocrPages": intp(8)}, {"catalogReads": intp(20)}, {"dedupeJobs": nil}, {}, {"ocrThreads": intp(4), "dedupeJobs": intp(4)}}
	for _, c := range ok {
		if err := performance.Validate(c, m); err != nil {
			t.Errorf("Validate(%v) = %v, want ok", c, err)
		}
	}
	bad := []map[string]*int{{"ocrPages": intp(0)}, {"ocrPages": intp(9)}, {"catalogReads": intp(21)}, {"dedupeJobs": intp(5)}, {"ocrThreads": intp(-1)}, {"sqlInjection": intp(1)}, {"": intp(1)}}
	for _, c := range bad {
		if err := performance.Validate(c, m); err == nil {
			t.Errorf("Validate(%v) = ok, want an error", c)
		}
	}
}

func TestWarnings(t *testing.T) {
	gb := int64(1 << 30)
	cases := []struct {
		name     string
		settings performance.Settings
		machine  performance.Machine
		want     []string
	}{
		{"the defaults ask nothing", performance.Defaults(8), performance.Machine{Cores: 4, MemoryBytes: 8 * gb}, nil},
		{"pages times cores above the cores", performance.Settings{"ocrPages": 4, "ocrThreads": 2}, performance.Machine{Cores: 4, MemoryBytes: 64 * gb}, []string{"ocrCores"}},
		{"exactly the cores is fine", performance.Settings{"ocrPages": 4, "ocrThreads": 1}, performance.Machine{Cores: 4, MemoryBytes: 64 * gb}, nil},
		{"the OCR above half the memory", performance.Settings{"ocrPages": 8, "ocrThreads": 1}, performance.Machine{Cores: 16, MemoryBytes: 4 * gb}, []string{"ocrMemory"}},
		{"both", performance.Settings{"ocrPages": 8, "ocrThreads": 2}, performance.Machine{Cores: 4, MemoryBytes: 4 * gb}, []string{"ocrCores", "ocrMemory"}},
		{"unknown machine says nothing", performance.Settings{"ocrPages": 8, "ocrThreads": 8}, performance.Machine{}, nil},
	}
	for _, c := range cases {
		got := performance.Warnings(c.settings, c.machine)
		if len(got) != len(c.want) {
			t.Errorf("%s: warnings = %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: warnings = %v, want %v", c.name, got, c.want)
			}
		}
	}
}

func TestEffective_TheOwnersChoiceWinsOverTheDefault(t *testing.T) {
	got := performance.Effective(performance.Defaults(8), map[string]int{"ocrPages": 3})
	if got["ocrPages"] != 3 || got["ocrThreads"] != 1 || got["catalogReads"] != 8 {
		t.Errorf("Effective = %v", got)
	}
}

func TestLive_FollowersHearTheChangeAndTheStartingValues(t *testing.T) {
	live := performance.NewLive(performance.Defaults(8))
	var seen []int
	live.OnChange(func(s performance.Settings) { seen = append(seen, s["catalogReads"]) })
	live.Apply(map[string]int{"catalogReads": 12})
	if live.Get("catalogReads") != 12 {
		t.Errorf("Get = %d, want 12", live.Get("catalogReads"))
	}
	if len(seen) != 2 || seen[0] != 8 || seen[1] != 12 {
		t.Errorf("a follower heard %v, want [8 12] (the current values at once, and the change)", seen)
	}
	live.Apply(map[string]int{"ocrPages": 2}) // only this one is chosen: the rest is the default again
	if live.Get("catalogReads") != 8 || live.Get("ocrPages") != 2 || live.Get("ocrThreads") != 1 {
		t.Errorf("after Apply: catalogReads=%d ocrPages=%d ocrThreads=%d", live.Get("catalogReads"), live.Get("ocrPages"), live.Get("ocrThreads"))
	}
	live.Apply(nil)
	if live.Get("ocrPages") != 1 {
		t.Errorf("with nothing chosen ocrPages = %d, want the default 1", live.Get("ocrPages"))
	}
}

func open(t *testing.T) *sql.DB {
	t.Helper()
	db := testdb.Open(t)
	if err := database.Migrate(db); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestSave_StoresOnlyWhatWasChosen_AndTheDefaultReturnsWithNil(t *testing.T) {
	db := open(t)
	m := performance.Machine{Cores: 8}
	got, err := performance.Overrides(ctx, db, m)
	if err != nil || len(got) != 0 {
		t.Fatalf("a fresh instance has no overrides: %v, %v", got, err)
	}
	if _, err := performance.Save(ctx, db, "", map[string]*int{"ocrPages": intp(3), "catalogReads": intp(12)}, m); err != nil {
		t.Fatal(err)
	}
	got, _ = performance.Overrides(ctx, db, m)
	if len(got) != 2 || got["ocrPages"] != 3 || got["catalogReads"] != 12 {
		t.Errorf("overrides = %v", got)
	}
	// a change of one field does not touch the others
	if _, err := performance.Save(ctx, db, "", map[string]*int{"ocrThreads": intp(2)}, m); err != nil {
		t.Fatal(err)
	}
	got, _ = performance.Overrides(ctx, db, m)
	if len(got) != 3 || got["ocrPages"] != 3 {
		t.Errorf("a change of ocrThreads lost the others: %v", got)
	}
	// nil goes back to the default: the field leaves the row
	left, err := performance.Save(ctx, db, "", map[string]*int{"ocrPages": nil}, m)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := left["ocrPages"]; ok || len(left) != 2 {
		t.Errorf("after the default of ocrPages came back: %v", left)
	}
	got, _ = performance.Overrides(ctx, db, m)
	if _, ok := got["ocrPages"]; ok {
		t.Errorf("ocrPages is still stored: %v", got)
	}
}

func TestSave_RefusesWhatIsOutOfRange_AndChangesNothing(t *testing.T) {
	db := open(t)
	m := performance.Machine{Cores: 4}
	if _, err := performance.Save(ctx, db, "", map[string]*int{"ocrPages": intp(2)}, m); err != nil {
		t.Fatal(err)
	}
	if _, err := performance.Save(ctx, db, "", map[string]*int{"ocrThreads": intp(2), "ocrPages": intp(99)}, m); err == nil {
		t.Fatal("99 pages on a machine of 4 cores was accepted")
	}
	got, _ := performance.Overrides(ctx, db, m)
	if len(got) != 1 || got["ocrPages"] != 2 {
		t.Errorf("a refused change changed something: %v", got)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM audit_log WHERE action = 'performance.update'`).Scan(&n)
	if n != 1 {
		t.Errorf("audit entries = %d, want 1 (the refused change leaves none)", n)
	}
}

func TestSave_IsAuditedWithWhatItWasAndWhatItBecame(t *testing.T) {
	db := open(t)
	m := performance.Machine{Cores: 8}
	performance.Save(ctx, db, "", map[string]*int{"ocrPages": intp(2)}, m)
	performance.Save(ctx, db, "", map[string]*int{"ocrPages": intp(4)}, m)
	var raw []byte
	if err := db.QueryRow(`SELECT details FROM audit_log WHERE action = 'performance.update' ORDER BY id DESC LIMIT 1`).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var d struct {
		Before map[string]int `json:"before"`
		After  map[string]int `json:"after"`
	}
	json.Unmarshal(raw, &d)
	if d.Before["ocrPages"] != 2 || d.After["ocrPages"] != 4 {
		t.Errorf("audit details = %s, want before 2 and after 4", raw)
	}
}

func TestOverrides_IgnoresAStoredValueThatIsNoLongerValid(t *testing.T) {
	db := open(t)
	// another version, or a smaller machine, left values the range does not allow
	db.Exec(`INSERT INTO settings (key, value) VALUES ('performance', '{"ocrPages": 64, "ocrThreads": 2, "gone": 3, "dedupeJobs": 0}')`)
	got, err := performance.Overrides(ctx, db, performance.Machine{Cores: 4})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got["ocrThreads"] != 2 {
		t.Errorf("overrides = %v, want only the valid ocrThreads", got)
	}
	// a row that is not an object at all is as if it was not there
	db.Exec(`UPDATE settings SET value = '"nonsense"'::jsonb WHERE key = 'performance'`)
	if got, err := performance.Overrides(ctx, db, performance.Machine{Cores: 4}); err != nil || len(got) != 0 {
		t.Errorf("a row that is not an object: %v, %v", got, err)
	}
}

func TestSave_TwoChangesAtOnceDoNotLoseOneAnother(t *testing.T) {
	db := open(t)
	m := performance.Machine{Cores: 8}
	var wg sync.WaitGroup
	for _, field := range []string{"ocrPages", "ocrThreads", "dedupeJobs", "catalogReads"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := performance.Save(ctx, db, "", map[string]*int{field: intp(2)}, m); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, _ := performance.Overrides(ctx, db, m)
	if len(got) != 4 {
		t.Errorf("four changes at once left %v, want all four", got)
	}
}
