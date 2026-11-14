package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestIndexSkipsFileOpenFailure(t *testing.T) {
	public := t.TempDir()
	at := int64(1793404800)
	g := testGroup()
	if e := AppendGroup(public, g, at, []any{at, 1}); e != nil {
		t.Fatal(e)
	}
	p := filepath.Join(public, "data/1m/2026-10-31/unreadable.jsonl")
	if e := os.Symlink("absent-target", p); e != nil {
		t.Fatal(e)
	}
	e := WriteIndex(public, Index{Schema: 1, Version: "0.1.0", GeneratedAt: at, Groups: []Group{g}})
	if e == nil || !strings.Contains(e.Error(), "unreadable.jsonl") {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(public, "index.json"))
	if e != nil {
		t.Fatal(e)
	}
	var idx Index
	if e = json.Unmarshal(b, &idx); e != nil || idx.GeneratedAt != at || len(idx.Groups) != 1 || idx.Groups[0].ID != "cpu" {
		t.Fatal(idx, e)
	}
	b, e = os.ReadFile(filepath.Join(public, "index.js"))
	if e != nil || !strings.HasPrefix(string(b), "HD.index(") {
		t.Fatal(string(b), e)
	}
}
