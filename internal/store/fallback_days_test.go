package store

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCoarseFallbackClearsMissingMinuteDays(t *testing.T) {
	public := t.TempDir()
	g := testGroup()
	at := int64(1793404800)
	if e := AppendGroup(public, g, at, []any{at, 2}); e != nil {
		t.Fatal(e)
	}
	h := g.Header()
	h.Step = 300
	h.Columns, h.Units, _ = coarseColumns(g.Columns, g.Units)
	p := filepath.Join(public, "data/5m/2026-10/cpu.jsonl")
	if e := appendRows(p, h, [][]any{{at, 1, at, 2, 2}}); e != nil {
		t.Fatal(e)
	}
	if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: at, Groups: []Group{g}}); e != nil {
		t.Fatal(e)
	}
	if e := os.RemoveAll(filepath.Join(public, "data/1m")); e != nil {
		t.Fatal(e)
	}
	injected := errors.New("injected coarse recovery I/O failure")
	called := false
	e := writeIndex(public, Index{Schema: 1, GeneratedAt: at + 60}, func([]coarseFile, Group) (Group, error) {
		called = true
		return Group{}, injected
	})
	if !called || !errors.Is(e, injected) {
		t.Fatal("expected recovery error", called, e)
	}
	b, e := os.ReadFile(filepath.Join(public, "index.json"))
	if e != nil {
		t.Fatal(e)
	}
	var idx Index
	if e := json.Unmarshal(b, &idx); e != nil {
		t.Fatal(e)
	}
	if len(idx.Groups) != 1 || idx.Groups[0].Days == nil || len(idx.Groups[0].Days) != 0 {
		t.Fatal(string(b))
	}
}
