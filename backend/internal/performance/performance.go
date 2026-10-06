// Package performance is what the owner may tune from the administration, without editing the .env or restarting anything:
// how many heavy reads of the catalog run at once, how many comparisons of possible duplicates, how many pages of a scan the
// OCR reads at once and how many cores one page may use.
//
// Only what the owner changed is stored (`settings`, key "performance"): a value that was never touched is the one the
// installation starts with (the .env and the compose file), so a new default of a new version reaches whoever did not choose.
// Each service reads what it needs by itself and changes at the next job: a job that is running is never cut.
package performance

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/ocnaibill/codice/backend/internal/audit"
)

// Key is the row of `settings` that holds what the owner chose.
const Key = "performance"

// Field names, as they travel in the API and are stored.
const (
	CatalogReads = "catalogReads" // heavy reads of the catalog at once (the list, the counters, the search, the favorites)
	DedupeJobs   = "dedupeJobs"   // comparisons of possible duplicates at once
	OCRPages     = "ocrPages"     // pages of a scanned PDF that the OCR reads at once
	OCRThreads   = "ocrThreads"   // cores that the engine may use for one page
)

// Fields in the order the screen shows them.
var Fields = []string{CatalogReads, DedupeJobs, OCRPages, OCRThreads}

// Settings are the values in use.
type Settings map[string]int

// Range is the least and the most a value can have.
type Range struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// Defaults are what the installation starts with. The catalog limit comes from CODICE_CATALOG_CONCURRENCY (already read and
// checked at the start); the rest start at one, as it has always been.
func Defaults(catalogReads int) Settings {
	if catalogReads <= 0 {
		catalogReads = 8
	}
	return Settings{CatalogReads: catalogReads, DedupeJobs: 1, OCRPages: 1, OCRThreads: 1}
}

// Machine is what the process can see of the machine it runs on.
type Machine struct {
	Cores       int   `json:"cores"`
	MemoryBytes int64 `json:"memoryBytes"` // 0 when it cannot be told
}

// ThisMachine reads the cores the process may use and the memory its container may use (the cgroup limit, else the
// memory of the host).
func ThisMachine() Machine {
	return Machine{Cores: runtime.NumCPU(), MemoryBytes: memoryLimit()}
}

func memoryLimit() int64 {
	for _, path := range []string{"/sys/fs/cgroup/memory.max", "/sys/fs/cgroup/memory/memory.limit_in_bytes"} {
		if data, err := os.ReadFile(path); err == nil {
			if n, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64); err == nil && n > 0 && n < 1<<50 {
				return n
			}
		}
	}
	if data, err := os.ReadFile("/proc/meminfo"); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if rest, ok := strings.CutPrefix(line, "MemTotal:"); ok {
				fields := strings.Fields(rest)
				if len(fields) > 0 {
					if kb, err := strconv.ParseInt(fields[0], 10, 64); err == nil {
						return kb * 1024
					}
				}
			}
		}
	}
	return 0
}

// Limits are the least and the most each value can have on a machine: more than the cores it has makes nothing faster,
// and the catalog reads cannot take more than a pool of connections has.
func Limits(m Machine) map[string]Range {
	cores := max(1, m.Cores)
	return map[string]Range{
		CatalogReads: {Min: 1, Max: 20},
		DedupeJobs:   {Min: 1, Max: min(cores, 4)},
		OCRPages:     {Min: 1, Max: min(cores, 8)},
		OCRThreads:   {Min: 1, Max: min(cores, 8)},
	}
}

// OCRPageMemory is what one page that is being read costs in memory, about (the engine and the picture of the page): the
// measure of the first real round was 336 MiB with the OCR running.
const OCRPageMemory = 340 << 20

// Warnings are the codes of what the values in use ask more of the machine than it has; the screen says them in words.
func Warnings(s Settings, m Machine) []string {
	var out []string
	if cores := m.Cores; cores > 0 && s[OCRPages]*s[OCRThreads] > cores {
		out = append(out, "ocrCores") // the pages at once times the cores of each is more than the machine has
	}
	if m.MemoryBytes > 0 && int64(s[OCRPages])*OCRPageMemory > m.MemoryBytes/2 {
		out = append(out, "ocrMemory") // the OCR alone would take more than half of the memory
	}
	return out
}

// Validate checks a change: it names the field that is out of its range or unknown. A nil value means "back to the default".
func Validate(changes map[string]*int, m Machine) error {
	limits := Limits(m)
	for field, value := range changes {
		r, ok := limits[field]
		if !ok {
			return fmt.Errorf("unknown setting %q", field)
		}
		if value != nil && (*value < r.Min || *value > r.Max) {
			return fmt.Errorf("%s must be from %d to %d", field, r.Min, r.Max)
		}
	}
	return nil
}

// Overrides reads what the owner chose (only the fields that are set, and valid ones: a value stored by another version
// that is out of its range is ignored, as if it was not there).
func Overrides(ctx context.Context, db *sql.DB, m Machine) (map[string]int, error) {
	var raw []byte
	err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, Key).Scan(&raw)
	if err == sql.ErrNoRows {
		return map[string]int{}, nil
	}
	if err != nil {
		return nil, err
	}
	var stored map[string]int
	if err := json.Unmarshal(raw, &stored); err != nil {
		return map[string]int{}, nil
	}
	limits := Limits(m)
	out := map[string]int{}
	for field, value := range stored {
		if r, ok := limits[field]; ok && value >= r.Min && value <= r.Max {
			out[field] = value
		}
	}
	return out, nil
}

// Effective is the values in use: what the owner chose over the defaults.
func Effective(defaults Settings, overrides map[string]int) Settings {
	out := Settings{}
	for field, value := range defaults {
		out[field] = value
	}
	for field, value := range overrides {
		out[field] = value
	}
	return out
}

// Save applies a change to what is stored, records who did it and what changed, and returns the overrides that remain.
// `changes` has the field and its new value, or nil to go back to the default.
func Save(ctx context.Context, db *sql.DB, actor string, changes map[string]*int, m Machine) (map[string]int, error) {
	if err := Validate(changes, m); err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// One change at a time, even the first one, when there is no row yet for FOR UPDATE to hold.
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext('performance'))`); err != nil {
		return nil, err
	}
	var raw []byte
	err = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = $1`, Key).Scan(&raw)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	stored := map[string]int{}
	if len(raw) > 0 {
		_ = json.Unmarshal(raw, &stored)
	}
	before := map[string]int{}
	for k, v := range stored {
		before[k] = v
	}
	for field, value := range changes {
		if value == nil {
			delete(stored, field)
		} else {
			stored[field] = *value
		}
	}
	value, _ := json.Marshal(stored)
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_by) VALUES ($1, $2::jsonb, NULLIF($3, '')::uuid)
		ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = now()`,
		Key, value, actor); err != nil {
		return nil, err
	}
	_ = audit.Record(ctx, tx, actor, "performance.update", "settings", Key, map[string]any{"before": before, "after": stored})
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return stored, nil
}

// Live holds the values the process is using now and what has to follow them. The API starts it with what is stored, and the
// owner's change reaches it at once.
type Live struct {
	mu        sync.RWMutex
	defaults  Settings
	values    Settings
	overrides map[string]int
	onChange  []func(Settings)
}

// NewLive starts at the defaults.
func NewLive(defaults Settings) *Live {
	return &Live{defaults: Effective(defaults, nil), values: Effective(defaults, nil)}
}

// Get is the value of a field now.
func (l *Live) Get(field string) int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.values[field]
}

// Overridden says whether the owner chose the value of a field (else it is the default of the installation).
func (l *Live) Overridden(field string) bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	_, ok := l.overrides[field]
	return ok
}

// OnChange registers what to do when the values change (and does it once now with the current ones).
func (l *Live) OnChange(f func(Settings)) {
	l.mu.Lock()
	l.onChange = append(l.onChange, f)
	current := Effective(l.values, nil)
	l.mu.Unlock()
	f(current)
}

// Apply puts what the owner chose over the defaults and tells whoever follows the values.
func (l *Live) Apply(overrides map[string]int) {
	l.mu.Lock()
	l.values = Effective(l.defaults, overrides)
	l.overrides = map[string]int{}
	for k, v := range overrides {
		l.overrides[k] = v
	}
	current := Effective(l.values, nil)
	callbacks := append([]func(Settings){}, l.onChange...)
	l.mu.Unlock()
	for _, f := range callbacks {
		f(current)
	}
}
