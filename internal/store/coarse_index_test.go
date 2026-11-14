package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestIndexCoarseFilesAndStoppedGroup(t *testing.T) {
	public := t.TempDir()
	at := int64(1793404800)
	g := testGroup()
	if e := AppendGroup(public, g, at, []any{at, 1}); e != nil {
		t.Fatal(e)
	}
	if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: at, Groups: []Group{g}}); e != nil {
		t.Fatal(e)
	}
	h := g.Header()
	h.Step = 300
	h.Columns, h.Units, _ = coarseColumns(g.Columns, g.Units)
	for _, month := range []string{"2026-09", "2026-10"} {
		if e := appendRows(filepath.Join(public, "data/5m", month, "cpu.jsonl"), h, [][]any{{at - 300, 2, at - 60, 1, 2}}); e != nil {
			t.Fatal(e)
		}
	}
	os.MkdirAll(filepath.Join(public, "data/1h/2026-10"), 0755)
	os.WriteFile(filepath.Join(public, "data/1h/2026-10/cpu.jsonl"), nil, 0644)
	// Listing must use stat without attempting to parse the coarse file.
	bad := filepath.Join(public, "data/1h/2026-09/cpu.jsonl")
	os.MkdirAll(filepath.Dir(bad), 0755)
	os.WriteFile(bad, []byte("not JSON\n"), 0644)
	if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: at}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(public, "index.json"))
	var idx Index
	json.Unmarshal(b, &idx)
	if len(idx.Resolutions) != 4 || idx.CoarseColumns != "time is the UTC-aligned interval start; samples is the number of 1-minute rows in the interval (rows with all values null included); last_time is the latest 1-minute time in the interval; then for each 1m column c: c (average of non-null 1-minute values, rounded to 3 decimals) and c_max (maximum 1-minute value). Maxima of different columns may come from different minutes; do not add them. Ratios use means, never maxima; do not compute a ratio when either material mean is zero and its corresponding maximum is positive (rounding)." {
		t.Fatal(string(b))
	}
	if !reflect.DeepEqual(idx.Groups[0].Coarse, map[string][]string{"5m": {"2026-09", "2026-10"}, "1h": {"2026-09"}, "1d": {}}) {
		t.Fatal(string(b))
	}
	if e := os.RemoveAll(filepath.Join(public, "data/1m")); e != nil {
		t.Fatal(e)
	}
	if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: at}); e != nil {
		t.Fatal(e)
	}
	b, _ = os.ReadFile(filepath.Join(public, "index.json"))
	json.Unmarshal(b, &idx)
	if len(idx.Groups) != 1 {
		t.Fatal(string(b))
	}
	got := idx.Groups[0]
	if got.Days == nil || len(got.Days) != 0 || got.LastSeen != at-60 || !reflect.DeepEqual(got.Columns, g.Columns) || !reflect.DeepEqual(got.Units, g.Units) || !reflect.DeepEqual(got.Descriptions, g.Descriptions) {
		t.Fatal(got)
	}
	if got.Traffic != nil {
		t.Fatal("traffic on non-net group")
	}
}

func TestCoarseOnlyIndexUsesLatestHeaderAndLastObservation(t *testing.T) {
	public := t.TempDir()
	g := testGroup()
	h := g.Header()
	h.Step = 86400
	h.Columns, h.Units, _ = coarseColumns(g.Columns, g.Units)
	p := filepath.Join(public, "data/1d/2026/cpu.jsonl")
	if e := appendRows(p, h, [][]any{{1793318400, 1, 1793318460, 2, 2}}); e != nil {
		t.Fatal(e)
	}
	h.Name = "new name"
	h.Columns = append(h.Columns, "idle", "idle_max")
	h.Units = append(h.Units, "%", "%")
	if e := appendRows(p, h, [][]any{{1793404800, 1, 1793404860, 3, 3, 97, 97}}); e != nil {
		t.Fatal(e)
	}
	for _, suffix := range []string{"\n", "[999"} {
		f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
		f.WriteString(suffix)
		f.Close()
	}
	if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: 1793491200}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(public, "index.json"))
	var idx Index
	json.Unmarshal(b, &idx)
	if len(idx.Groups) != 1 || idx.Groups[0].LastSeen != 1793404860 || idx.Groups[0].Name != "new name" || !reflect.DeepEqual(idx.Groups[0].Columns, []string{"time", "user", "idle"}) || !reflect.DeepEqual(idx.Groups[0].Descriptions, []string{"time", "user", "idle"}) {
		t.Fatal(string(b))
	}
}

func TestIndexDoesNotRetainDeletedCoarseInventory(t *testing.T) {
	public := t.TempDir()
	g := testGroup()
	at := int64(1793404800)
	if e := AppendGroup(public, g, at, []any{at, 1}); e != nil {
		t.Fatal(e)
	}
	h := g.Header()
	h.Step = 300
	h.Columns, h.Units, _ = coarseColumns(g.Columns, g.Units)
	if e := appendRows(filepath.Join(public, "data/5m/2026-10/cpu.jsonl"), h, [][]any{{at, 1, at, 1, 1}}); e != nil {
		t.Fatal(e)
	}
	if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: at}); e != nil {
		t.Fatal(e)
	}
	if e := os.RemoveAll(filepath.Join(public, "data/5m")); e != nil {
		t.Fatal(e)
	}
	if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: at}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(public, "index.json"))
	var idx Index
	json.Unmarshal(b, &idx)
	if len(idx.Groups) != 1 || len(idx.Groups[0].Coarse["5m"]) != 0 {
		t.Fatal(string(b))
	}
}
