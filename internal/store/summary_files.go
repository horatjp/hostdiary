// summary_files.go — writes public summary.json and its private cache.
//
// Completed days are cached in summary-cache.json beside public/ and reused
// while the day file's size and mtime are unchanged, so each completed day is
// normally parsed once after midnight; today's (and future) files are parsed
// on every collection.
package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type SummaryMeta struct {
	Version     string
	Host        string
	GeneratedAt int64
}
type SummaryDay struct {
	Day             string     `json:"day"`
	Status          string     `json:"status"`
	ExpectedMinutes *int       `json:"expected_minutes"`
	Groups          []GroupDay `json:"groups"`
}
type Summary struct {
	Schema      int          `json:"schema"`
	Version     string       `json:"version"`
	Host        string       `json:"host"`
	GeneratedAt int64        `json:"generated_at"`
	Guide       string       `json:"guide"`
	Days        []SummaryDay `json:"days"`
}
type summaryCacheEntry struct {
	Size    int64    `json:"size"`
	MtimeNS int64    `json:"mtime_ns"`
	Summary GroupDay `json:"summary"`
}
type summaryCache struct {
	Schema  int                          `json:"schema"`
	Entries map[string]summaryCacheEntry `json:"entries"`
}

const summaryCacheName = "summary-cache.json"

// loadSummaryCache treats a missing, unreadable or foreign cache as empty:
// the cache only saves work and never changes the result.
func loadSummaryCache(path string) ([]byte, summaryCache) {
	empty := summaryCache{Schema: 1, Entries: map[string]summaryCacheEntry{}}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, empty
	}
	var c summaryCache
	if json.Unmarshal(b, &c) != nil || c.Schema != 1 || c.Entries == nil {
		return b, empty
	}
	return b, c
}

// WriteSummary rebuilds summary.json from the retained minute files. Read
// errors skip the affected day or file and are returned after writing.
func WriteSummary(public string, meta SummaryMeta) error {
	private := filepath.Dir(public)
	cachePath := filepath.Join(private, summaryCacheName)
	oldCache, cache := loadSummaryCache(cachePath)
	next := summaryCache{Schema: 1, Entries: map[string]summaryCacheEntry{}}
	out := Summary{Schema: 1, Version: meta.Version, Host: meta.Host, GeneratedAt: meta.GeneratedAt, Guide: "llms.txt", Days: []SummaryDay{}}
	var failures []error
	root := filepath.Join(public, "data", "1m")
	days, e := os.ReadDir(root)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	for _, day := range days {
		if !day.IsDir() || !ValidDay(day.Name()) {
			continue
		}
		date, _ := time.Parse("2006-01-02", day.Name())
		status, expected := dayStatus(date.Unix(), meta.GeneratedAt)
		files, e := os.ReadDir(filepath.Join(root, day.Name()))
		if e != nil {
			failures = append(failures, fmt.Errorf("summary day %s: %w", day.Name(), e))
			continue
		}
		sd := SummaryDay{Day: day.Name(), Status: status, ExpectedMinutes: expected, Groups: []GroupDay{}}
		for _, file := range files {
			if !file.Type().IsRegular() || filepath.Ext(file.Name()) != ".jsonl" {
				continue
			}
			p := filepath.Join(root, day.Name(), file.Name())
			key := day.Name() + "/" + strings.TrimSuffix(file.Name(), ".jsonl")
			st, e := os.Stat(p)
			if e != nil {
				failures = append(failures, fmt.Errorf("summary %s: %w", p, e))
				continue
			}
			if c, ok := cache.Entries[key]; ok && status == "complete" && c.Size == st.Size() && c.MtimeNS == st.ModTime().UnixNano() {
				next.Entries[key] = c
				sd.Groups = append(sd.Groups, c.Summary)
				continue
			}
			h, rows, e := readMinuteDay(p)
			if e != nil {
				failures = append(failures, fmt.Errorf("summary %s: %w", p, e))
				continue
			}
			if h.Schema != 1 || len(rows) == 0 {
				continue // Headers only or unsupported: no data to summarize.
			}
			g, e := summarizeGroupDay(h, rows, date.Unix(), meta.GeneratedAt)
			if e != nil {
				failures = append(failures, fmt.Errorf("summary %s: %w", p, e))
				continue
			}
			if g.Rows == 0 {
				continue // Every row was outside the file's day.
			}
			if status == "complete" {
				next.Entries[key] = summaryCacheEntry{st.Size(), st.ModTime().UnixNano(), g}
			}
			sd.Groups = append(sd.Groups, g)
		}
		sort.Slice(sd.Groups, func(i, j int) bool { return sd.Groups[i].ID < sd.Groups[j].ID })
		out.Days = append(out.Days, sd)
	}
	b, e := json.Marshal(out)
	if e != nil {
		return errors.Join(append(failures, e)...)
	}
	if e = AtomicWrite(filepath.Join(public, "summary.json"), append(b, '\n'), 0644, private); e != nil {
		failures = append(failures, e)
	}
	// Rewrite the cache only when completed days changed (about once a day).
	if cb, e := json.Marshal(next); e == nil && !bytes.Equal(bytes.TrimSpace(oldCache), cb) {
		if e = AtomicWrite(cachePath, append(cb, '\n'), 0600, private); e != nil {
			failures = append(failures, fmt.Errorf("summary cache: %w", e))
		}
	}
	return errors.Join(failures...)
}
