package collect

import (
	"encoding/json"
	"hostdiary/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Rollup failure preserves source days, but not at the expense of index/state progress.
func TestRollupFailureRetainsNineDaysThenCatchesUp(t *testing.T) {
	root := copyFixture(t)
	public := filepath.Join(root, "var/lib/hostdiary/public")
	day := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	g := store.Group{ID: "cpu", Kind: "cpu", Name: "cpu", Columns: []string{"time", "user"}, Units: []string{"s", "%"}}
	for i := 0; i < 9; i++ {
		at := day.AddDate(0, 0, i).Unix()
		if e := store.AppendGroup(public, g, at, []any{at, 2}); e != nil {
			t.Fatal(e)
		}
	}
	blocker := filepath.Join(public, "data/5m/2026-10/cpu.js")
	if e := os.MkdirAll(blocker, 0755); e != nil {
		t.Fatal(e)
	}
	at := day.AddDate(0, 0, 9).Unix() + 43200
	if code, log := runAt(root, at); code != 1 || !strings.Contains(log, "rollup") {
		t.Fatal(code, log)
	}
	oldest := filepath.Join(public, "data/1m/2026-10-01")
	if _, e := os.Stat(oldest); e != nil {
		t.Fatal("oldest source deleted", e)
	}
	if index(t, root).GeneratedAt != at {
		t.Fatal("index did not advance")
	}
	state, e := loadState(filepath.Join(root, "var/lib/hostdiary/state.json"))
	if e != nil || state.LastCollected != at {
		t.Fatal(state, e)
	}
	if e := os.Remove(blocker); e != nil {
		t.Fatal(e)
	}
	if code, log := runAt(root, at+60); code != 0 {
		t.Fatal(code, log)
	}
	if _, e := os.Stat(oldest); !os.IsNotExist(e) {
		t.Fatal("oldest source not pruned", e)
	}
	for _, res := range []struct{ name, period string }{{"5m", "2026-10"}, {"1h", "2026-10"}, {"1d", "2026"}} {
		b, e := os.ReadFile(filepath.Join(public, "data", res.name, res.period, "cpu.jsonl"))
		if e != nil {
			t.Fatal(e)
		}
		count := 0
		first := int64(0)
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "[") {
				var row []json.RawMessage
				if e := json.Unmarshal([]byte(line), &row); e != nil {
					t.Fatal(e)
				}
				if count == 0 {
					json.Unmarshal(row[0], &first)
				}
				count++
			}
		}
		if count != 9 || first != day.Unix() {
			t.Fatal(res.name, count, first)
		}
	}
	if code, log := runAt(root, at+120); code != 0 {
		t.Fatal(code, log)
	}
	if index(t, root).GeneratedAt != at+120 {
		t.Fatal("index did not advance after retry")
	}
}
