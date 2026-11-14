package store

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Unreadable formats are absent data; only I/O failures may stop this group.
func TestRollupSkipsDaysWithoutReadableRows(t *testing.T) {
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	header := func(change func(*Header)) string {
		h := testGroup().Header()
		change(&h)
		b, e := json.Marshal(h)
		if e != nil {
			t.Fatal(e)
		}
		return string(b) + "\n"
	}
	valid := header(func(*Header) {})
	row := "[1790812800,99]\n"
	for name, input := range map[string]string{
		"empty":                         "",
		"no header":                     row,
		"unsupported schema":            header(func(h *Header) { h.Schema = 2 }) + row,
		"unsupported step":              header(func(h *Header) { h.Step = 300 }) + row,
		"bad columns":                   header(func(h *Header) { h.Columns[0] = "other" }) + row,
		"bad units":                     header(func(h *Header) { h.Units = nil }) + row,
		"header only":                   valid,
		"truncated header":              strings.TrimSuffix(valid, "\n"),
		"invalid header replaces valid": valid + "{}\n" + row,
	} {
		t.Run(name, func(t *testing.T) {
			public := t.TempDir()
			p := filepath.Join(public, "data/1m/2026-10-01/cpu.jsonl")
			if e := os.MkdirAll(filepath.Dir(p), 0755); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(p, []byte(input), 0644); e != nil {
				t.Fatal(e)
			}
			minuteFixture(t, public, day.AddDate(0, 0, 1), testGroup())
			for range 2 {
				if e := Rollup(public, day.AddDate(0, 0, 2)); e != nil {
					t.Fatal(e)
				}
				for _, res := range []struct {
					name, period string
					count        int
				}{{"5m", "2026-10", 288}, {"1h", "2026-10", 24}, {"1d", "2026", 1}} {
					rows := coarseRows(t, filepath.Join(public, "data", res.name, res.period, "cpu.jsonl"))
					if len(rows) != res.count || rows[0][0] != float64(day.AddDate(0, 0, 1).Unix()) {
						t.Fatal(res.name, len(rows), rows[0])
					}
				}
			}
		})
	}
}

func TestReadMinuteDayInvalidHeadersNeverReusePreviousColumns(t *testing.T) {
	valid, _ := json.Marshal(testGroup().Header())
	for name, bad := range map[string]string{
		"wrong schema":  `{"schema":2,"step":60,"columns":["time","user"],"units":["s","%"]}`,
		"wrong step":    `{"schema":1,"step":300,"columns":["time","user"],"units":["s","%"]}`,
		"invalid units": `{"schema":1,"step":60,"columns":["time","user"]}`,
	} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "cpu.jsonl")
			input := string(valid) + "\n[1790812800,2]\n" + bad + "\n[1790812860,999]\n" + string(valid) + "\n[1790812920,4]\n" + bad + "\n[1790812980,999]\n"
			if e := os.WriteFile(p, []byte(input), 0644); e != nil {
				t.Fatal(e)
			}
			h, rows, e := readMinuteDay(p)
			if e != nil || len(rows) != 2 || h.Schema != 1 || h.Step != 60 {
				t.Fatal(h, rows, e)
			}
			if *rows[0].Values["user"] != 2 || *rows[1].Values["user"] != 4 {
				t.Fatal(rows)
			}
		})
	}
}

func TestAggregateNormalizesRoundedZero(t *testing.T) {
	b, e := aggregate([]namedRow{{Time: 0, Values: map[string]*float64{"x": ptr(-0.0004)}}}, 300, 0)
	if e != nil {
		t.Fatal(e)
	}
	mean := *b[0].Mean["x"]
	if mean != 0 || math.Signbit(mean) {
		t.Fatalf("expected positive zero, got %v", mean)
	}
	raw, e := json.Marshal(mean)
	if e != nil || string(raw) != "0" {
		t.Fatal(string(raw), e)
	}
}

func TestRollupUnreadableDayRecordsErrorAndStopsLaterDays(t *testing.T) {
	public := t.TempDir()
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	other := testGroup()
	other.ID = "other"
	for i := range 3 {
		for _, g := range []Group{testGroup(), other} {
			minuteFixture(t, public, day.AddDate(0, 0, i), g)
		}
	}
	unreadable := filepath.Join(public, "data/1m/2026-10-02")
	injected := errors.New("earlier group failure")
	changed := false
	e := rollup(public, day.AddDate(0, 0, 3), func(p string, h Header, rows [][]any) error {
		if !changed {
			// Replace after root ReadDir, so the directory read fails on every OS/user.
			if e := os.RemoveAll(unreadable); e != nil {
				t.Fatal(e)
			}
			if e := os.WriteFile(unreadable, []byte("not a directory"), 0644); e != nil {
				t.Fatal(e)
			}
			changed = true
		}
		if h.Group == "cpu" {
			return injected
		}
		return appendRows(p, h, rows)
	})
	if !errors.Is(e, injected) || !strings.Contains(e.Error(), "rollup day 2026-10-02") {
		t.Fatal(e)
	}
	if len(coarseRows(t, filepath.Join(public, "data/1d/2026/other.jsonl"))) != 1 {
		t.Fatal("other group of earlier day did not finish")
	}
	for _, id := range []string{"cpu", "other"} {
		p := filepath.Join(public, "data/5m/2026-10", id+".jsonl")
		if id == "cpu" {
			if _, e := os.Stat(p); !os.IsNotExist(e) {
				t.Fatal(e)
			}
			continue
		}
		if rows := coarseRows(t, p); len(rows) != 288 || rows[len(rows)-1][0] >= float64(day.AddDate(0, 0, 1).Unix()) {
			t.Fatal("later day advanced", len(rows))
		}
	}
}

func TestReadMinuteDayPropagatesIOErrors(t *testing.T) {
	for _, p := range []string{filepath.Join(t.TempDir(), "missing"), t.TempDir()} {
		if _, _, e := readMinuteDay(p); e == nil {
			t.Fatal("expected I/O error", p)
		}
	}
}
