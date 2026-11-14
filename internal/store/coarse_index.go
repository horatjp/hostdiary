// Coarse inventories use directory entries/stat; rows are decoded only for stopped-group recovery.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type resolution struct {
	Name string `json:"name"`
	Step int    `json:"step"`
	Path string `json:"path"`
	Keep string `json:"keep"`
}

const coarseColumnsDescription = "time is the UTC-aligned interval start; samples is the number of 1-minute rows in the interval (rows with all values null included); last_time is the latest 1-minute time in the interval; then for each 1m column c: c (average of non-null 1-minute values, rounded to 3 decimals) and c_max (maximum 1-minute value). Maxima of different columns may come from different minutes; do not add them. Ratios use means, never maxima; do not compute a ratio when either material mean is zero and its corresponding maximum is positive (rounding)."

func resolutions() []resolution {
	return []resolution{
		{"1m", 60, "data/1m/{day}/{id}.jsonl", "at least 8 newest UTC days (more while rollup is failing)"},
		{"5m", 300, "data/5m/{month}/{id}.jsonl", "3 newest UTC months"},
		{"1h", 3600, "data/1h/{month}/{id}.jsonl", "25 newest UTC months"},
		{"1d", 86400, "data/1d/{year}/{id}.jsonl", "3 newest UTC years"},
	}
}

type coarseFile struct{ resolution, period, path string }

func listCoarse(public string) (map[string][]coarseFile, error) {
	out := map[string][]coarseFile{}
	var failures []error
	for _, res := range resolutions()[1:] {
		root := filepath.Join(public, "data", res.Name)
		periods, e := os.ReadDir(root)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil {
			failures = append(failures, e)
			continue
		}
		layout := "2006-01"
		if res.Name == "1d" {
			layout = "2006"
		}
		for _, p := range periods {
			date, e := time.Parse(layout, p.Name())
			if !p.IsDir() || e != nil || date.Year() < 1 || date.Format(layout) != p.Name() {
				continue
			}
			files, e := os.ReadDir(filepath.Join(root, p.Name()))
			if e != nil {
				failures = append(failures, e)
				continue
			}
			for _, f := range files {
				if !f.Type().IsRegular() || filepath.Ext(f.Name()) != ".jsonl" {
					continue
				}
				info, e := f.Info()
				if e != nil {
					failures = append(failures, e)
					continue
				}
				if info.Size() == 0 {
					continue
				}
				id := strings.TrimSuffix(f.Name(), ".jsonl")
				out[id] = append(out[id], coarseFile{res.Name, p.Name(), filepath.Join(root, p.Name(), f.Name())})
			}
		}
	}
	return out, errors.Join(failures...)
}
func coarsePeriods(files []coarseFile) map[string][]string {
	out := map[string][]string{"5m": {}, "1h": {}, "1d": {}}
	for _, f := range files {
		out[f.resolution] = append(out[f.resolution], f.period)
	}
	for _, periods := range out {
		sort.Strings(periods)
	}
	return out
}

func recoverCoarseGroup(files []coarseFile, previous Group) (Group, error) {
	latest := map[string]coarseFile{}
	for _, f := range files {
		if f.period > latest[f.resolution].period {
			latest[f.resolution] = f
		}
	}
	var best Header
	var last int64
	var failures []error
	for _, res := range resolutions()[1:] {
		f, ok := latest[res.Name]
		if !ok {
			continue
		}
		h, t, e := readCoarseSummary(f.path, res.Step)
		if e != nil {
			failures = append(failures, fmt.Errorf("index coarse %s: %w", f.path, e))
			continue
		}
		if h.Group != "" && t > last {
			best = h
			last = t
		}
	}
	if best.Group == "" {
		return Group{}, errors.Join(failures...)
	}
	g := Group{ID: best.Group, Kind: best.Kind, Name: best.Name, LastSeen: last, Days: []string{}}
	for i, c := range best.Columns {
		if c == "samples" || c == "last_time" || strings.HasSuffix(c, "_max") {
			continue
		}
		g.Columns = append(g.Columns, c)
		g.Units = append(g.Units, best.Units[i])
	}
	descriptions := map[string]string{}
	for i, c := range previous.Columns {
		if i < len(previous.Descriptions) {
			descriptions[c] = previous.Descriptions[i]
		}
	}
	for _, c := range g.Columns {
		d, ok := descriptions[c]
		if !ok {
			d = c
		}
		g.Descriptions = append(g.Descriptions, d)
	}
	return g, errors.Join(failures...)
}

// Scan backwards for the last complete row and its header. Intermediate data rows
// are skipped without decoding. Trailing headers may change metadata, not last_seen.
func readCoarseSummary(path string, step int) (Header, int64, error) {
	f, e := os.Open(path)
	if e != nil {
		return Header{}, 0, e
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil {
		return Header{}, 0, e
	}
	end := st.Size()
	tail := true
	var carry []byte
	var row []json.RawMessage
	var latest Header
	visit := func(b []byte) (int64, bool) {
		b = bytes.TrimSpace(b)
		if len(b) == 0 {
			return 0, false
		}
		if b[0] == '[' && row == nil {
			var r []json.RawMessage
			if json.Unmarshal(b, &r) == nil && len(r) > 2 {
				row = r
			}
			return 0, false
		}
		if b[0] != '{' {
			return 0, false
		}
		var h Header
		if json.Unmarshal(b, &h) != nil || h.Schema != 1 || h.Step != step || len(h.Columns) != len(h.Units) {
			return 0, false
		}
		if latest.Group == "" {
			latest = h
		}
		if row != nil {
			if len(row) != len(h.Columns) {
				return 0, true // Do not reinterpret the row with an older header.
			}
			for i, c := range h.Columns {
				if c == "last_time" && i < len(row) {
					var t int64
					if json.Unmarshal(row[i], &t) == nil {
						return t, true
					}
				}
			}
		}
		return 0, false
	}
	for end > 0 {
		start := max(int64(0), end-4096)
		chunk := make([]byte, end-start)
		_, e := f.ReadAt(chunk, start)
		if e != nil && e != io.EOF {
			return Header{}, 0, e
		}
		parts := bytes.Split(append(chunk, carry...), []byte{'\n'})
		for i := len(parts) - 1; i >= 1; i-- {
			if tail {
				tail = false
				continue
			}
			if t, ok := visit(parts[i]); ok {
				return latest, t, nil
			}
		}
		carry = parts[0]
		end = start
	}
	if !tail {
		if t, ok := visit(carry); ok {
			return latest, t, nil
		}
	}
	return latest, 0, nil
}
