package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCoarseColumnsContract(t *testing.T) {
	c, u, e := coarseColumns([]string{"time", "x"}, []string{"s", "%"})
	if e != nil || !reflect.DeepEqual(c, []string{"time", "samples", "last_time", "x", "x_max"}) || !reflect.DeepEqual(u, []string{"s", "count", "s", "%", "%"}) {
		t.Fatal(c, u, e)
	}
	for _, c := range [][]string{nil, {"x"}, {"time", "samples"}, {"time", "x_max"}} {
		if _, _, e := coarseColumns(c, make([]string, len(c))); e == nil {
			t.Fatal(c)
		}
	}
	if _, _, e := coarseColumns([]string{"time"}, nil); e == nil {
		t.Fatal("unit mismatch")
	}
}

func TestAggregateContract(t *testing.T) {
	rows := []namedRow{{-1, map[string]*float64{"x": ptr(99)}}, {0, map[string]*float64{"x": ptr(1)}}, {0, map[string]*float64{"x": ptr(2)}}, {299, map[string]*float64{"x": ptr(5)}}, {300, map[string]*float64{}}, {86400, map[string]*float64{"x": ptr(99)}}}
	before := *rows[1].Values["x"]
	for _, step := range []int64{300, 3600, 86400} {
		got, e := aggregate(rows, step, 0)
		if e != nil {
			t.Fatal(e)
		}
		if got[0].Time != 0 || got[0].Samples != map[bool]int{true: 2, false: 3}[step == 300] || *got[0].Mean["x"] != 3.5 || *got[0].Max["x"] != 5 {
			t.Fatal(got)
		}
		if step == 300 {
			if len(got) != 2 || got[0].LastTime != 299 || got[1].Mean["x"] != nil || got[1].LastTime != 300 {
				t.Fatal(got)
			}
		} else if len(got) != 1 || got[0].LastTime != 300 {
			t.Fatal(got)
		}
	}
	if *rows[1].Values["x"] != before {
		t.Fatal("mutated input")
	}
	for _, tc := range [][2]int64{{60, 0}, {300, 1}} {
		if _, e := aggregate(nil, tc[0], tc[1]); e == nil {
			t.Fatal(tc)
		}
	}
	got, _ := aggregate([]namedRow{{0, map[string]*float64{"x": ptr(1)}}, {1, map[string]*float64{"x": ptr(2)}}, {2, map[string]*float64{"x": ptr(2)}}}, 300, 0)
	if *got[0].Mean["x"] != 1.667 || got[0].Samples != 3 {
		t.Fatal(got)
	}
}
func ptr(v float64) *float64 { return &v }

func TestAppendRowsContract(t *testing.T) {
	public := t.TempDir()
	p := filepath.Join(public, "data/5m/2026-10/cpu.jsonl")
	h := testGroup().Header()
	h.Step = 300
	if e := appendRows(p, h, nil); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Dir(p)); !os.IsNotExist(e) {
		t.Fatal("empty append created directory")
	}
	if e := appendRows(p, h, [][]any{{1}}); e == nil {
		t.Fatal("row length")
	}
	if e := appendRows(p, h, [][]any{{1, 2}}); e != nil {
		t.Fatal(e)
	}
	for _, ext := range []string{".jsonl", ".js"} {
		path := strings.TrimSuffix(p, ".jsonl") + ext
		before, _ := os.ReadFile(path)
		fail := func(path string, b []byte) error {
			f, e := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
			if e != nil {
				return e
			}
			f.Write(b[:len(b)/2])
			f.Close()
			return errors.New("injected")
		}
		if e := appendRowsWith(p, h, [][]any{{2, 3}, {3, 4}}, func(path string, b []byte) error {
			if strings.HasSuffix(path, ext) {
				return fail(path, b)
			}
			return writeBatch(path, b)
		}); e == nil {
			t.Fatal("failure missing")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal(ext)
		}
	}
	for _, ext := range []string{".jsonl", ".js"} {
		b, _ := os.ReadFile(strings.TrimSuffix(p, ".jsonl") + ext)
		if bytes.Count(b, []byte{'\n'}) != 2 {
			t.Fatal(string(b))
		}
	}
}

func TestAppendRowsSingleWriteAndRepairBeforeRollback(t *testing.T) {
	for _, failedExt := range []string{".jsonl", ".js"} {
		p := filepath.Join(t.TempDir(), "data/5m/2026-10/cpu.jsonl")
		h := testGroup().Header()
		h.Step = 300
		if e := appendRows(p, h, [][]any{{1, 2}}); e != nil {
			t.Fatal(e)
		}
		before := map[string][]byte{}
		for _, ext := range []string{".jsonl", ".js"} {
			path := strings.TrimSuffix(p, ".jsonl") + ext
			b, _ := os.ReadFile(path)
			before[path] = b
			os.WriteFile(path, append(b, []byte("partial")...), 0644)
		}
		h.Name = "updated"
		calls := map[string]int{}
		e := appendRowsWith(p, h, [][]any{{2, 3}, {3, 4}}, func(path string, b []byte) error {
			calls[filepath.Ext(path)]++
			if strings.HasSuffix(path, failedExt) {
				writeBatch(path, b[:len(b)/2])
				return errors.New("injected")
			}
			return writeBatch(path, b)
		})
		if e == nil {
			t.Fatal("expected failure")
		}
		for path, b := range before {
			got, _ := os.ReadFile(path)
			if !bytes.Equal(got, b) {
				t.Fatal(path)
			}
		}
		for _, n := range calls {
			if n != 1 {
				t.Fatal(calls)
			}
		}
		calls = map[string]int{}
		if e := appendRowsWith(p, h, [][]any{{2, 3}, {3, 4}}, func(path string, b []byte) error { calls[filepath.Ext(path)]++; return writeBatch(path, b) }); e != nil {
			t.Fatal(e)
		}
		if calls[".js"] != 1 || calls[".jsonl"] != 1 {
			t.Fatal(calls)
		}
		for path := range before {
			b, _ := os.ReadFile(path)
			if bytes.Count(b, []byte{'\n'}) != 5 {
				t.Fatal(string(b))
			}
		}
	}
}

func TestAggregateDoesNotCapSamples(t *testing.T) {
	rows := []namedRow{}
	for i := 0; i < 10; i++ {
		rows = append(rows, namedRow{int64(i), map[string]*float64{"x": ptr(2)}})
	}
	b, e := aggregate(rows, 300, 0)
	if e != nil || len(b) != 1 || b[0].Samples != 10 {
		t.Fatal(b, e)
	}
}

func TestLastTimeCompleteDataOnly(t *testing.T) {
	p := filepath.Join(t.TempDir(), "tail.jsonl")
	for _, tail := range []string{"", "[99", "{}\n[99", strings.Repeat("x", 10000)} {
		os.WriteFile(p, []byte("[123,1]\n{}\n"+tail), 0644)
		if got, e := lastTime(p); e != nil || got != 123 {
			t.Fatal(got, e)
		}
	}
	if got, e := lastTime(p + "missing"); got != 0 || e != nil {
		t.Fatal(got, e)
	}
}

func TestCoarseRetentionAndRepair(t *testing.T) {
	public := t.TempDir()
	for _, tc := range []struct {
		res, layout string
		keep        int
	}{{"1m", "2006-01-02", 8}, {"5m", "2006-01", 3}, {"1h", "2006-01", 25}, {"1d", "2006", 3}} {
		root := filepath.Join(public, "data", tc.res)
		for i := 0; i < tc.keep+1; i++ {
			name := time.Date(2020+i, 1, 1, 0, 0, 0, 0, time.UTC).Format(tc.layout)
			dir := filepath.Join(root, name)
			os.MkdirAll(dir, 0755)
			for _, ext := range []string{".js", ".jsonl"} {
				os.WriteFile(filepath.Join(dir, "cpu"+ext), []byte("ok\ncut"), 0644)
			}
		}
		for _, name := range []string{"notes", "2026-13", "0000"} {
			dir := filepath.Join(root, name)
			os.MkdirAll(dir, 0755)
			os.WriteFile(filepath.Join(dir, "cpu.js"), []byte("untouched"), 0644)
		}
	}
	if e := RepairAndPrune(public, true); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(public, "data/1m/2020-01-01")); e != nil {
		t.Fatal("skip minute prune", e)
	}
	if e := RepairAndPrune(public, false); e != nil {
		t.Fatal(e)
	}
	for _, res := range []string{"1m", "5m", "1h", "1d"} {
		entries, _ := os.ReadDir(filepath.Join(public, "data", res))
		want := map[string]int{"1m": 8, "5m": 3, "1h": 25, "1d": 3}[res] + 3
		if len(entries) != want {
			t.Fatal(res, len(entries), want)
		}
		for _, d := range entries {
			b, _ := os.ReadFile(filepath.Join(public, "data", res, d.Name(), "cpu.js"))
			if string(b) != "ok\n" && string(b) != "untouched" {
				t.Fatal(res, d.Name(), string(b))
			}
		}
	}
}
