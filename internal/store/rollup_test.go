package store

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func minuteFixture(t *testing.T, public string, day time.Time, g Group) {
	t.Helper()
	// Write fixture directly: minute appends are intentionally unchanged, not under test here.
	h, _ := json.Marshal(g.Header())
	var b bytes.Buffer
	b.Write(h)
	b.WriteByte('\n')
	for i := 0; i < 1440; i++ {
		r, _ := json.Marshal([]any{day.Unix() + int64(i*60), float64(i)})
		b.Write(r)
		b.WriteByte('\n')
	}
	p := filepath.Join(public, "data/1m", day.Format("2006-01-02"), g.ID+".jsonl")
	os.MkdirAll(filepath.Dir(p), 0755)
	if e := os.WriteFile(p, b.Bytes(), 0644); e != nil {
		t.Fatal(e)
	}
}
func coarseRows(t *testing.T, p string) [][]float64 {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	out := [][]float64{}
	for _, line := range bytes.Split(b, []byte{'\n'}) {
		if len(line) > 0 && line[0] == '[' {
			var r []float64
			if e := json.Unmarshal(line, &r); e != nil {
				continue // Readers ignore incomplete or malformed rows.
			}
			out = append(out, r)
		}
	}
	return out
}
func treeBytes(t *testing.T, public string) map[string]string {
	t.Helper()
	out := map[string]string{}
	e := filepath.WalkDir(public, func(p string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if !d.IsDir() {
			b, e := os.ReadFile(p)
			if e != nil {
				return e
			}
			out[p] = string(b)
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	return out
}

func TestRollupGenerationAndIdempotence(t *testing.T) {
	public := t.TempDir()
	day := time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 4; i++ {
		minuteFixture(t, public, day.AddDate(0, 0, i), testGroup())
	}
	// Noncanonical directories must not participate.
	os.MkdirAll(filepath.Join(public, "data/1m/notes"), 0755)
	at := day.AddDate(0, 0, 2)
	if e := Rollup(public, at); e != nil {
		t.Fatal(e)
	}
	for _, res := range []struct {
		name, period string
		n            int
		mean, peak   float64
	}{{"5m", "2026-10", 576, 2, 4}, {"1h", "2026-10", 48, 29.5, 59}, {"1d", "2026", 2, 719.5, 1439}} {
		p := filepath.Join(public, "data", res.name, res.period, "cpu.jsonl")
		rows := coarseRows(t, p)
		if len(rows) != res.n || rows[0][3] != res.mean || rows[0][4] != res.peak || rows[0][1] != float64(2880/res.n) {
			t.Fatal(res.name, len(rows), rows[0])
		}
		if rows[len(rows)-1][2] != float64(day.AddDate(0, 0, 2).Unix()-60) {
			t.Fatal(rows[len(rows)-1])
		}
		b, _ := os.ReadFile(strings.TrimSuffix(p, ".jsonl") + ".js")
		if !bytes.Contains(b, []byte(`HD.l("`+res.name+`/`+res.period+`/cpu",`)) {
			t.Fatal(string(b[:100]))
		}
	}
	before := treeBytes(t, public)
	if e := Rollup(public, at); e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(before, treeBytes(t, public)) {
		t.Fatal("not idempotent")
	}
}

func TestRollupResumeAndFailureIsolation(t *testing.T) {
	for _, failure := range []string{"5m", "1h", "1d"} {
		t.Run(failure, func(t *testing.T) {
			public := t.TempDir()
			day := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
			other := testGroup()
			other.ID = "other"
			for i := 0; i < 2; i++ {
				for _, g := range []Group{testGroup(), other} {
					minuteFixture(t, public, day.AddDate(0, 0, i), g)
				}
			}
			failed := errors.New("injected")
			e := rollup(public, day.AddDate(0, 0, 2), func(p string, h Header, r [][]any) error {
				if h.Group == "cpu" && h.Step == map[string]int{"5m": 300, "1h": 3600, "1d": 86400}[failure] {
					return failed
				}
				return appendRows(p, h, r)
			})
			if !errors.Is(e, failed) {
				t.Fatal(e)
			}
			if _, e := os.Stat(filepath.Join(public, "data/5m/2027-01/cpu.jsonl")); !os.IsNotExist(e) {
				t.Fatal("later day written", e)
			}
			if len(coarseRows(t, filepath.Join(public, "data/1d/2027/other.jsonl"))) != 1 {
				t.Fatal("other group did not continue")
			}
			// Closed-period JS and JSONL tails must not prevent the retry.
			for _, ext := range []string{".jsonl", ".js"} {
				p := filepath.Join(public, "data/5m/2026-12/cpu"+ext)
				if b, e := os.ReadFile(p); e == nil {
					os.WriteFile(p, append(b, []byte("partial")...), 0644)
				}
			}
			if e := Rollup(public, day.AddDate(0, 0, 2)); e != nil {
				t.Fatal(e)
			}
			for _, year := range []string{"2026", "2027"} {
				if len(coarseRows(t, filepath.Join(public, "data/1d", year, "cpu.jsonl"))) != 1 {
					t.Fatal("daily duplicate")
				}
			}
			before := treeBytes(t, public)
			if e := Rollup(public, day.AddDate(0, 0, 2)); e != nil {
				t.Fatal(e)
			}
			if !reflect.DeepEqual(before, treeBytes(t, public)) {
				t.Fatal("retry duplicate")
			}
		})
	}
}

func TestRollupChangingHeadersAndTailResume(t *testing.T) {
	public := t.TempDir()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	at := day.Unix()
	g := testGroup()
	if e := AppendGroup(public, g, at, []any{at, 2}); e != nil {
		t.Fatal(e)
	}
	g.Columns = []string{"time", "new", "user"}
	g.Units = []string{"s", "count", "%"}
	if e := AppendGroup(public, g, at+60, []any{at + 60, 10, 4}); e != nil {
		t.Fatal(e)
	}
	if e := AppendGroup(public, g, at+60, []any{at + 60, 20, 6}); e != nil {
		t.Fatal(e)
	}
	// Exact duplicate keeps the later row; ranges outside this day are discarded.
	if e := AppendGroup(public, g, at+120, []any{at + 86400, 999, 999}); e != nil {
		t.Fatal(e)
	}
	if e := Rollup(public, day.AddDate(0, 0, 1)); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(public, "data/5m/2026-10/cpu.jsonl")
	r := coarseRows(t, p)
	if len(r) != 1 || !reflect.DeepEqual(r[0], []float64{float64(at), 2, float64(at + 60), 20, 20, 4, 6}) {
		t.Fatal(r)
	}
	// Simulate 5m committed before the next resolutions, with a complete header and incomplete row tail.
	os.RemoveAll(filepath.Join(public, "data/1h"))
	os.RemoveAll(filepath.Join(public, "data/1d"))
	b, _ := os.ReadFile(p)
	h, _ := lastHeader(p, ".jsonl")
	hb, _ := json.Marshal(h)
	b = append(b, hb...)
	b = append(b, '\n')
	b = append(b, []byte("[999")...)
	os.WriteFile(p, b, 0644)
	if e := Rollup(public, day.AddDate(0, 0, 1)); e != nil {
		t.Fatal(e)
	}
	if len(coarseRows(t, p)) != 1 || len(coarseRows(t, filepath.Join(public, "data/1h/2026-10/cpu.jsonl"))) != 1 {
		t.Fatal("resume")
	}
}

func TestFailedRollupRetainsMinuteSources(t *testing.T) {
	public := t.TempDir()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 9; i++ {
		minuteFixture(t, public, day.AddDate(0, 0, i), testGroup())
	}
	e := rollup(public, day.AddDate(0, 0, 9), func(string, Header, [][]any) error { return errors.New("injected") })
	if e == nil {
		t.Fatal("expected failure")
	}
	if e := RepairAndPrune(public, true); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(public, "data/1m/2026-10-01")); e != nil {
		t.Fatal(e)
	}
	if e := Rollup(public, day.AddDate(0, 0, 9)); e != nil {
		t.Fatal(e)
	}
	if e := RepairAndPrune(public, false); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(public, "data/1m/2026-10-01")); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	if len(coarseRows(t, filepath.Join(public, "data/1d/2026/cpu.jsonl"))) != 9 {
		t.Fatal("lost oldest day")
	}
}

func TestMonthEndBatchTiming(t *testing.T) {
	public := t.TempDir()
	h := testGroup().Header()
	h.Step = 300
	h.Columns, h.Units, _ = coarseColumns([]string{"time", "a", "b", "c", "d", "e", "f", "g", "h"}, []string{"s", "%", "%", "%", "%", "%", "%", "%", "%"})
	rows := make([][]any, 30*288)
	for i := range rows {
		rows[i] = []any{int64(i * 300), 5, int64(i*300 + 240)}
		for j := 0; j < 8; j++ {
			rows[i] = append(rows[i], 12.345, 98.765)
		}
	}
	paths := []string{}
	for i := 0; i < 20; i++ {
		p := filepath.Join(public, "data/5m/2026-10", string(rune('a'+i))+".jsonl")
		paths = append(paths, p)
		if e := appendRows(p, h, rows); e != nil {
			t.Fatal(e)
		}
	}
	before, _ := os.Stat(paths[0])
	start := time.Now()
	for _, p := range paths {
		if e := appendRows(p, h, rows[:288]); e != nil {
			t.Fatal(e)
		}
	}
	t.Logf("20 groups x 19 columns x (8640 existing rows (%d JSONL bytes/group) + 288 new rows), 2 mirrors: %s", before.Size(), time.Since(start))
}
