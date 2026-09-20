package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"
)

// Policy is how many packages to keep (DEC-064 suggests 7 daily, 4 weekly, 3 monthly).
type Policy struct{ Daily, Weekly, Monthly int }

// DefaultPolicy is the suggestion from DEC-064.
var DefaultPolicy = Policy{Daily: 7, Weekly: 4, Monthly: 3}

// PackageName is the file name a package is given, which is also how prune reads its age.
func PackageName(t time.Time, encrypted bool) string {
	name := "codice-backup-" + t.UTC().Format("20060102-150405") + ".tar"
	if encrypted {
		name += ".age"
	}
	return name
}

var packageRe = regexp.MustCompile(`^codice-backup-(\d{8}-\d{6})\.tar(\.age)?$`)

// PruneResult lists what is kept and what goes.
type PruneResult struct{ Keep, Delete []string }

// Prune chooses which packages in dir to keep: the newest of each of the last Daily days,
// Weekly weeks and Monthly months, and always the newest one. Only files named like
// packages are ever considered. With dryRun it changes nothing.
func Prune(dir string, p Policy, dryRun bool) (PruneResult, error) {
	var res PruneResult
	entries, err := os.ReadDir(dir)
	if err != nil {
		return res, err
	}
	type pkg struct {
		name string
		at   time.Time
	}
	var all []pkg
	for _, e := range entries {
		m := packageRe.FindStringSubmatch(e.Name())
		if m == nil || !e.Type().IsRegular() {
			continue
		}
		at, err := time.Parse("20060102-150405", m[1])
		if err != nil {
			continue
		}
		all = append(all, pkg{e.Name(), at})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].at.After(all[j].at) })

	keep := map[string]bool{}
	pick := func(limit int, bucket func(time.Time) string) {
		seen := map[string]bool{}
		for _, x := range all {
			b := bucket(x.at)
			if seen[b] {
				continue
			}
			if len(seen) >= limit {
				break
			}
			seen[b] = true
			keep[x.name] = true
		}
	}
	pick(p.Daily, func(t time.Time) string { return t.Format("2006-01-02") })
	pick(p.Weekly, func(t time.Time) string { y, w := t.ISOWeek(); return fmt.Sprintf("%d-%02d", y, w) })
	pick(p.Monthly, func(t time.Time) string { return t.Format("2006-01") })
	if len(all) > 0 {
		keep[all[0].name] = true
	}

	for _, x := range all {
		if keep[x.name] {
			res.Keep = append(res.Keep, x.name)
			continue
		}
		res.Delete = append(res.Delete, x.name)
		if !dryRun {
			if err := os.Remove(filepath.Join(dir, x.name)); err != nil {
				return res, err
			}
		}
	}
	return res, nil
}
