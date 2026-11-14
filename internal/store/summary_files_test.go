package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func readSummary(t *testing.T, public string) Summary {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(public, "summary.json"))
	if e != nil {
		t.Fatal(e)
	}
	var s Summary
	if e := json.Unmarshal(b, &s); e != nil {
		t.Fatal(e)
	}
	return s
}
func readCache(t *testing.T, public string) summaryCache {
	t.Helper()
	_, c := loadSummaryCache(filepath.Join(filepath.Dir(public), summaryCacheName))
	return c
}

// summaryFixture: a complete day (1440 rows, value = minute) and today with 3 rows.
func summaryFixture(t *testing.T) (string, time.Time, int64) {
	t.Helper()
	public := filepath.Join(t.TempDir(), "public")
	day := time.Date(2026, 11, 7, 0, 0, 0, 0, time.UTC)
	minuteFixture(t, public, day, testGroup())
	today := day.AddDate(0, 0, 1)
	p := filepath.Join(public, "data/1m/2026-11-08/cpu.jsonl")
	os.MkdirAll(filepath.Dir(p), 0755)
	h, _ := json.Marshal(testGroup().Header())
	body := string(h) + "\n"
	for i, v := range []int{5, 1, 9} {
		r, _ := json.Marshal([]any{today.Unix() + int64(i*60), v})
		body += string(r) + "\n"
	}
	body += "[1794096180,\n" // partial tail line is ignored
	if e := os.WriteFile(p, []byte(body), 0644); e != nil {
		t.Fatal(e)
	}
	return public, day, today.Unix() + 3*60
}

func TestWriteSummaryContents(t *testing.T) {
	public, day, at := summaryFixture(t)
	if e := WriteSummary(public, SummaryMeta{"0.1.0", "fixture-host", at}); e != nil {
		t.Fatal(e)
	}
	s := readSummary(t, public)
	if s.Schema != 1 || s.Version != "0.1.0" || s.Host != "fixture-host" || s.GeneratedAt != at || s.Guide != "llms.txt" || len(s.Days) != 2 {
		t.Fatalf("summary: %+v", s)
	}
	done, today := s.Days[0], s.Days[1]
	if done.Day != "2026-11-07" || done.Status != "complete" || *done.ExpectedMinutes != 1440 || len(done.Groups) != 1 {
		t.Fatalf("complete day: %+v", done)
	}
	g := done.Groups[0]
	u := column(g, "user")
	if g.Rows != 1440 || *g.MissingMinutes != 0 || g.First != day.Unix() || *u.Max != 1439 || *u.MaxTime != day.Unix()+1439*60 || *u.Mean != 719.5 {
		t.Fatalf("complete group: %+v %+v", g, u)
	}
	if today.Status != "partial" || *today.ExpectedMinutes != 4 || today.Groups[0].Rows != 3 || *today.Groups[0].MissingMinutes != 1 || *column(today.Groups[0], "user").Min != 1 {
		t.Fatalf("today: %+v", today)
	}
	if st, _ := os.Stat(filepath.Join(public, "summary.json")); st.Mode().Perm() != 0644 {
		t.Fatalf("summary mode %v", st.Mode())
	}
	c := readCache(t, public)
	if len(c.Entries) != 1 || c.Entries["2026-11-07/cpu"].Summary.Rows != 1440 {
		t.Fatalf("only completed days are cached: %+v", c.Entries)
	}
	if st, _ := os.Stat(filepath.Join(filepath.Dir(public), summaryCacheName)); st.Mode().Perm() != 0600 {
		t.Fatalf("cache mode %v", st.Mode())
	}
}

func TestWriteSummaryReusesCacheUntilFileChanges(t *testing.T) {
	public, _, at := summaryFixture(t)
	meta := SummaryMeta{"0.1.0", "h", at}
	if e := WriteSummary(public, meta); e != nil {
		t.Fatal(e)
	}
	cachePath := filepath.Join(filepath.Dir(public), summaryCacheName)
	c := readCache(t, public)
	entry := c.Entries["2026-11-07/cpu"]
	entry.Summary.Rows = 999 // Marker: visible only if the cache is reused.
	c.Entries["2026-11-07/cpu"] = entry
	b, _ := json.Marshal(c)
	os.WriteFile(cachePath, b, 0600)
	if e := WriteSummary(public, meta); e != nil {
		t.Fatal(e)
	}
	if r := readSummary(t, public).Days[0].Groups[0].Rows; r != 999 {
		t.Fatalf("unchanged completed day should come from the cache, rows=%d", r)
	}
	p := filepath.Join(public, "data/1m/2026-11-07/cpu.jsonl")
	later := time.Now().Add(time.Hour)
	os.Chtimes(p, later, later)
	if e := WriteSummary(public, meta); e != nil {
		t.Fatal(e)
	}
	if r := readSummary(t, public).Days[0].Groups[0].Rows; r != 1440 {
		t.Fatalf("changed file should be summarized again, rows=%d", r)
	}
}

func TestWriteSummaryDoesNotReusePartialOrBrokenCache(t *testing.T) {
	public, day, _ := summaryFixture(t)
	// While 2026-11-07 is today, nothing is cached and missing minutes follow at.
	at := day.Unix() + 3600
	if e := WriteSummary(public, SummaryMeta{"0.1.0", "h", at}); e != nil {
		t.Fatal(e)
	}
	if len(readCache(t, public).Entries) != 0 {
		t.Fatal("partial days must not be cached")
	}
	s := readSummary(t, public)
	if s.Days[0].Status != "partial" || s.Days[1].Status != "future" || s.Days[1].ExpectedMinutes != nil || s.Days[1].Groups[0].MissingMinutes != nil {
		t.Fatalf("statuses: %+v", s.Days)
	}
	os.WriteFile(filepath.Join(filepath.Dir(public), summaryCacheName), []byte("{broken"), 0600)
	if e := WriteSummary(public, SummaryMeta{"0.1.0", "h", day.Unix() + 86400 + 180}); e != nil {
		t.Fatal(e)
	}
	if g := readSummary(t, public).Days[0].Groups[0]; g.Rows != 1440 || *g.MissingMinutes != 0 {
		t.Fatalf("broken cache is ignored: %+v", g)
	}
	if len(readCache(t, public).Entries) != 1 {
		t.Fatal("broken cache is replaced")
	}
}

func TestWriteSummaryWithoutData(t *testing.T) {
	public := filepath.Join(t.TempDir(), "public")
	os.MkdirAll(public, 0755)
	if e := WriteSummary(public, SummaryMeta{"0.1.0", "h", 1794096000}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(public, "summary.json"))
	var raw map[string]any
	json.Unmarshal(b, &raw)
	if d, ok := raw["days"].([]any); !ok || len(d) != 0 {
		t.Fatalf("days must be an empty array: %s", b)
	}
}
