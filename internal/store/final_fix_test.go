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

func TestRepairTailPreservesCompleteFilesModTime(t *testing.T) {
	for _, contents := range []string{"", "complete\n", strings.Repeat("x", 9000) + "\n"} {
		p := filepath.Join(t.TempDir(), "history.jsonl")
		if e := os.WriteFile(p, []byte(contents), 0644); e != nil {
			t.Fatal(e)
		}
		old := time.Unix(1600000000, 0)
		if e := os.Chtimes(p, old, old); e != nil {
			t.Fatal(e)
		}
		before, e := os.Stat(p)
		if e != nil {
			t.Fatal(e)
		}
		if e := RepairTail(p); e != nil {
			t.Fatal(e)
		}
		after, e := os.Stat(p)
		if e != nil {
			t.Fatal(e)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
		if string(b) != contents || !before.ModTime().Equal(after.ModTime()) {
			t.Fatal("complete file changed", before.ModTime(), after.ModTime())
		}
	}
}

func TestIndexEmptyCoarseRecoveryReturnsErrorWithoutChangingFallback(t *testing.T) {
	for _, previous := range []bool{false, true} {
		t.Run(map[bool]string{true: "previous", false: "no previous"}[previous], func(t *testing.T) {
			public := t.TempDir()
			g := testGroup()
			h := g.Header()
			h.Step = 300
			h.Columns, h.Units, _ = coarseColumns(g.Columns, g.Units)
			p := filepath.Join(public, "data/5m/2026-10/cpu.jsonl")
			if e := appendRows(p, h, [][]any{{1793404800, 1, 1793404860, 2, 2}}); e != nil {
				t.Fatal(e)
			}
			if previous {
				if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: 1793491200, Groups: []Group{g}}); e != nil {
					t.Fatal(e)
				}
			}
			hb, _ := json.Marshal(h)
			if e := os.WriteFile(p, append(hb, '\n'), 0644); e != nil {
				t.Fatal(e)
			}
			e := WriteIndex(public, Index{Schema: 1, GeneratedAt: 1793491200})
			if e == nil || !strings.Contains(e.Error(), "index coarse cpu: no readable rows") {
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
				return
			}
			if len(idx.Groups) != 1 || idx.Groups[0].LastSeen != 1793404860 || !reflect.DeepEqual(idx.Groups[0].Descriptions, g.Descriptions) || len(idx.Groups[0].Days) != 0 {
				t.Fatal(string(b))
			}
		})
	}
}

type observedBatch struct {
	events  []string
	syncErr error
}

func (f *observedBatch) Write(b []byte) (int, error) {
	f.events = append(f.events, "write")
	return len(b), nil
}
func (f *observedBatch) Sync() error  { f.events = append(f.events, "sync"); return f.syncErr }
func (f *observedBatch) Close() error { f.events = append(f.events, "close"); return nil }

func TestCoarseBatchSyncsBeforeClose(t *testing.T) {
	for _, res := range []string{"5m", "1h", "1d"} {
		t.Run(res, func(t *testing.T) {
			f := &observedBatch{}
			p := filepath.Join("data", res, "2026-10", "cpu.jsonl")
			if e := writeBatchOpening(p, []byte("row\n"), func(string) (batchFile, error) { return f, nil }); e != nil {
				t.Fatal(e)
			}
			want := []string{"write", "sync", "close"}
			if res == "1d" {
				want = []string{"write", "close"}
			}
			if !reflect.DeepEqual(f.events, want) {
				t.Fatal(f.events, want)
			}
		})
	}
	f := &observedBatch{syncErr: errors.New("sync failure")}
	if e := writeBatchOpening("data/5m/2026-10/cpu.js", []byte("row\n"), func(string) (batchFile, error) { return f, nil }); !errors.Is(e, f.syncErr) || !reflect.DeepEqual(f.events, []string{"write", "sync", "close"}) {
		t.Fatal(e, f.events)
	}
}

type failingSyncFile struct {
	*os.File
	fail error
}

func (f failingSyncFile) Sync() error { return f.fail }

func TestCoarseSyncFailureRollsBackBothMirrors(t *testing.T) {
	p := filepath.Join(t.TempDir(), "data/5m/2026-10/cpu.jsonl")
	h := testGroup().Header()
	h.Step = 300
	if e := appendRows(p, h, [][]any{{1, 2}}); e != nil {
		t.Fatal(e)
	}
	before := map[string][]byte{}
	for _, ext := range []string{".jsonl", ".js"} {
		path := strings.TrimSuffix(p, ".jsonl") + ext
		b, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		before[path] = b
	}
	injected := errors.New("injected sync failure")
	e := appendRowsWith(p, h, [][]any{{2, 3}}, func(path string, b []byte) error {
		return writeBatchOpening(path, b, func(path string) (batchFile, error) {
			f, e := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0644)
			if e != nil {
				return nil, e
			}
			if strings.HasSuffix(path, ".js") {
				return failingSyncFile{f, injected}, nil
			}
			return f, nil
		})
	})
	if !errors.Is(e, injected) {
		t.Fatal(e)
	}
	for path, want := range before {
		got, e := os.ReadFile(path)
		if e != nil || !bytes.Equal(got, want) {
			t.Fatal(path, e)
		}
	}
}
