package store

import (
	"os"
	"path/filepath"
	"sort"
	"time"
)

// skipMinutePrune leaves source days available for retry after a failed rollup.
func RepairAndPrune(public string, skipMinutePrune bool) error {
	if e := repairMinuteAndPrune(public, skipMinutePrune); e != nil {
		return e
	}
	for _, res := range []struct {
		name, layout string
		keep         int
	}{{"5m", "2006-01", 3}, {"1h", "2006-01", 25}, {"1d", "2006", 3}} {
		root := filepath.Join(public, "data", res.name)
		entries, e := os.ReadDir(root)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			return e
		}
		names := []string{}
		for _, d := range entries {
			parsed, e := time.Parse(res.layout, d.Name())
			if d.IsDir() && e == nil && parsed.Year() > 0 && parsed.Format(res.layout) == d.Name() {
				names = append(names, d.Name())
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
		for i, name := range names {
			dir := filepath.Join(root, name)
			if i >= res.keep {
				if e := os.RemoveAll(dir); e != nil {
					return e
				}
				continue
			}
			files, e := os.ReadDir(dir)
			if e != nil {
				return e
			}
			for _, f := range files {
				ext := filepath.Ext(f.Name())
				if f.Type().IsRegular() && (ext == ".jsonl" || ext == ".js") {
					if e := RepairTail(filepath.Join(dir, f.Name())); e != nil {
						return e
					}
				}
			}
		}
	}
	return nil
}

func repairMinuteAndPrune(public string, skipPrune bool) error {
	root := filepath.Join(public, "data", "1m")
	entries, e := os.ReadDir(root)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	names := []string{}
	for _, d := range entries {
		if d.IsDir() {
			names = append(names, d.Name())
		}
	}
	var expired []string
	if !skipPrune {
		expired = ExpiredDays(names, 8)
	}
	for _, d := range expired {
		if e := os.RemoveAll(filepath.Join(root, d)); e != nil {
			return e
		}
	}
	// Only canonical day directories and recognized data extensions are owned here.
	return filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if p == root {
			return nil
		}
		if d.IsDir() {
			if !ValidDay(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type().IsRegular() && (filepath.Ext(p) == ".js" || filepath.Ext(p) == ".jsonl") {
			return RepairTail(p)
		}
		return nil
	})
}
