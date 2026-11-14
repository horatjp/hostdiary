package store

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func testGroup() Group {
	return Group{ID: "cpu", Kind: "cpu", Name: "cpu", Columns: []string{"time", "user"}, Units: []string{"s", "%"}, Descriptions: []string{"Unix seconds", "user CPU"}}
}
func TestAppendGroupHeadersAndMirror(t *testing.T) {
	public := t.TempDir()
	g := testGroup()
	at := int64(1793404800)
	for i := range 2 {
		if e := AppendGroup(public, g, at+int64(i), []any{at + int64(i), 1.125}); e != nil {
			t.Fatal(e)
		}
	}
	dir := filepath.Join(public, "data/1m/2026-10-31")
	for _, ext := range []string{".jsonl", ".js"} {
		f, e := os.OpenFile(filepath.Join(dir, "cpu"+ext), os.O_APPEND|os.O_WRONLY, 0)
		if e != nil {
			t.Fatal(e)
		}
		f.WriteString("broken(")
		f.Close()
	}
	g.Columns = append(g.Columns, "idle")
	g.Units = append(g.Units, "%")
	g.Descriptions = append(g.Descriptions, "idle CPU")
	if e := AppendGroup(public, g, at+2, []any{at + 2, 1.125, 98.875}); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(filepath.Join(dir, "cpu.jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	js, _ := os.ReadFile(filepath.Join(dir, "cpu.js"))
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	mirror := strings.Split(strings.TrimSpace(string(js)), "\n")
	if len(lines) != 5 || len(mirror) != 5 {
		t.Fatal(string(raw), string(js))
	}
	for i, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatal(line)
		}
		if mirror[i] != `HD.l("1m/2026-10-31/cpu",`+line+`);` {
			t.Fatal(mirror[i])
		}
	}
	if node, e := exec.LookPath("node"); e == nil {
		code := `let n=0;const HD={l:()=>n++};` + string(js) + `;if(n!==5) process.exit(1);`
		if b, e := exec.Command(node, "-e", code).CombinedOutput(); e != nil {
			t.Fatal(e, string(b))
		}
	} else {
		t.Log("Node unavailable; mirror checked structurally")
	}
}
func TestRepairAllDaysAndRetention(t *testing.T) {
	public := t.TempDir()
	root := filepath.Join(public, "data/1m")
	for _, day := range []string{"2026-10-01", "2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2099-01-01", "notes"} {
		dir := filepath.Join(root, day)
		os.MkdirAll(dir, 0755)
		for _, ext := range []string{".jsonl", ".js"} {
			os.WriteFile(filepath.Join(dir, "stopped"+ext), []byte("complete\ncut"), 0644)
		}
	}
	if e := RepairAndPrune(public, false); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(filepath.Join(root, "2026-10-01")); !os.IsNotExist(e) {
		t.Fatal(e)
	}
	for _, ext := range []string{".jsonl", ".js"} {
		b, e := os.ReadFile(filepath.Join(root, "2026-10-02", "stopped"+ext))
		if e != nil || string(b) != "complete\n" {
			t.Fatal(string(b), e)
		}
	}
	b, _ := os.ReadFile(filepath.Join(root, "notes/stopped.js"))
	if string(b) != "complete\ncut" {
		t.Fatal("modified unowned directory")
	}
}
func TestIndexRetainsStoppedGroups(t *testing.T) {
	public := t.TempDir()
	at := int64(1793404800)
	g := testGroup()
	if e := AppendGroup(public, g, at, []any{at, 5}); e != nil {
		t.Fatal(e)
	}
	idx := Index{Schema: 1, Version: "0.1.0", Host: "fixture-host", GeneratedAt: at, Groups: []Group{g}}
	for _, offset := range []int64{0, 60} {
		idx.GeneratedAt = at + offset
		if offset > 0 {
			idx.Groups = nil
		}
		if e := WriteIndex(public, idx); e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(filepath.Join(public, "index.json"))
		if e != nil {
			t.Fatal(e)
		}
		var got Index
		if e = json.Unmarshal(b, &got); e != nil {
			t.Fatal(e)
		}
		if len(got.Groups) != 1 || got.Groups[0].LastSeen != at || !reflect.DeepEqual(got.Groups[0].Descriptions, g.Descriptions) || got.GeneratedAt != at+offset {
			t.Fatalf("%+v", got)
		}
		js, _ := os.ReadFile(filepath.Join(public, "index.js"))
		if string(js) != "HD.index("+strings.TrimSpace(string(b))+");\n" {
			t.Fatal(string(js))
		}
	}
}
