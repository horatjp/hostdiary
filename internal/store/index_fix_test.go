package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Failed recovery must retain previous metadata, but always rebuild file inventories.
func TestIndexCoarseRecoveryFailurePreservesPreviousGroup(t *testing.T) {
	for _, previous := range []bool{true, false} {
		t.Run(map[bool]string{true: "previous", false: "no previous"}[previous], func(t *testing.T) {
			public := t.TempDir()
			g := testGroup()
			h := g.Header()
			h.Step = 300
			h.Columns, h.Units, _ = coarseColumns(g.Columns, g.Units)
			for _, month := range []string{"2026-09", "2026-10"} {
				if e := appendRows(filepath.Join(public, "data/5m", month, "cpu.jsonl"), h, [][]any{{1793404800, 1, 1793404860, 2, 2}}); e != nil {
					t.Fatal(e)
				}
			}
			if previous {
				if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: 1793491200, Groups: []Group{g}}); e != nil {
					t.Fatal(e)
				}
			}
			if e := os.RemoveAll(filepath.Join(public, "data/5m/2026-09")); e != nil {
				t.Fatal(e)
			}
			p := filepath.Join(public, "data/5m/2026-10/cpu.jsonl")
			if e := os.Chmod(p, 0); e != nil {
				t.Fatal(e)
			}
			defer os.Chmod(p, 0644)
			if f, e := os.Open(p); e == nil {
				f.Close()
				t.Skip("permission denial cannot be reproduced by this user")
			}
			for range 2 {
				e := WriteIndex(public, Index{Schema: 1, GeneratedAt: 1793491260})
				if e == nil || !strings.Contains(e.Error(), "index coarse") {
					t.Fatal(e)
				}
				b, e := os.ReadFile(filepath.Join(public, "index.json"))
				if e != nil {
					t.Fatal(e)
				}
				var idx Index
				if e := json.Unmarshal(b, &idx); e != nil {
					t.Fatal(e)
				}
				if !previous {
					if len(idx.Groups) != 0 {
						t.Fatal(string(b))
					}
					continue
				}
				if len(idx.Groups) != 1 {
					t.Fatal(string(b))
				}
				got := idx.Groups[0]
				if got.ID != "cpu" || got.LastSeen != 1793404860 || !reflect.DeepEqual(got.Descriptions, g.Descriptions) || !reflect.DeepEqual(got.Columns, g.Columns) || !reflect.DeepEqual(got.Units, g.Units) || got.Days == nil || len(got.Days) != 0 {
					t.Fatal(got)
				}
				if !reflect.DeepEqual(got.Coarse, map[string][]string{"5m": {"2026-10"}, "1h": {}, "1d": {}}) {
					t.Fatal(got.Coarse)
				}
				js, e := os.ReadFile(filepath.Join(public, "index.js"))
				if e != nil || !strings.Contains(string(js), `"descriptions":["Unix seconds","user CPU"]`) {
					t.Fatal(string(js), e)
				}
			}
		})
	}
}

func TestCoarseSummaryRejectsMismatchedRowLength(t *testing.T) {
	h := testGroup().Header()
	h.Step = 300
	h.Columns, h.Units, _ = coarseColumns(h.Columns, h.Units)
	hb, _ := json.Marshal(h)
	for _, row := range []string{"[1793404800,1,1793404860]", "[1793404800,1,1793404860,2,2,99]"} {
		p := filepath.Join(t.TempDir(), "cpu.jsonl")
		if e := os.WriteFile(p, []byte(string(hb)+"\n"+row+"\n"), 0644); e != nil {
			t.Fatal(e)
		}
		_, last, e := readCoarseSummary(p, 300)
		if e != nil || last != 0 {
			t.Fatal(last, e)
		}
	}
}

func TestIndexRejectsRecoveredGroupIDMismatch(t *testing.T) {
	public := t.TempDir()
	h := testGroup().Header()
	h.Step = 300
	h.Columns, h.Units, _ = coarseColumns(h.Columns, h.Units)
	for _, id := range []string{"cpu", "other"} {
		if e := appendRows(filepath.Join(public, "data/5m/2026-10", id+".jsonl"), h, [][]any{{1793404800, 1, 1793404860, 2, 2}}); e != nil {
			t.Fatal(e)
		}
	}
	e := WriteIndex(public, Index{Schema: 1, GeneratedAt: 1793491200})
	if e == nil || !strings.Contains(e.Error(), "other") || !strings.Contains(e.Error(), "cpu") {
		t.Fatal(e)
	}
	b, e := os.ReadFile(filepath.Join(public, "index.json"))
	if e != nil {
		t.Fatal(e)
	}
	var idx Index
	json.Unmarshal(b, &idx)
	if len(idx.Groups) != 1 || idx.Groups[0].ID != "cpu" {
		t.Fatal(string(b))
	}
}
