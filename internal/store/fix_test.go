package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendGroupRollback(t *testing.T) {
	for _, ext := range []string{".jsonl", ".js"} {
		t.Run(ext, func(t *testing.T) {
			public := t.TempDir()
			at := int64(1793404800)
			g := testGroup()
			if e := AppendGroup(public, g, at, []any{at, 1}); e != nil {
				t.Fatal(e)
			}
			dir := filepath.Join(public, "data/1m/2026-10-31")
			before := map[string][]byte{}
			for _, suffix := range []string{".jsonl", ".js"} {
				b, e := os.ReadFile(filepath.Join(dir, "cpu"+suffix))
				if e != nil {
					t.Fatal(e)
				}
				before[suffix] = b
			}
			injected := errors.New("injected append failure")
			write := func(p string, b []byte) error {
				if strings.HasSuffix(p, ext) {
					f, e := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0)
					if e != nil {
						return e
					}
					_, e = f.Write([]byte("partial"))
					f.Close()
					if e != nil {
						return e
					}
					return injected
				}
				return AppendLine(p, b)
			}
			if e := appendGroup(public, g, at+60, []any{at + 60, 2}, write); !errors.Is(e, injected) {
				t.Fatal(e)
			}
			for suffix, want := range before {
				got, e := os.ReadFile(filepath.Join(dir, "cpu"+suffix))
				if e != nil || !bytes.Equal(got, want) {
					t.Fatal(suffix, string(got), e)
				}
			}
		})
	}
}

func TestUTCDateBoundary(t *testing.T) {
	public := t.TempDir()
	g := testGroup()
	at := time.Date(2026, 10, 31, 23, 59, 59, 0, time.UTC).Unix()
	for _, delta := range []int64{0, 1} {
		if e := AppendGroup(public, g, at+delta, []any{at + delta, 1}); e != nil {
			t.Fatal(e)
		}
	}
	for _, day := range []string{"2026-10-31", "2026-11-01"} {
		for _, ext := range []string{".jsonl", ".js"} {
			b, e := os.ReadFile(filepath.Join(public, "data/1m", day, "cpu"+ext))
			if e != nil || bytes.Count(b, []byte{'\n'}) != 2 {
				t.Fatal(day, ext, string(b), e)
			}
		}
	}
}

func TestNewGroupRollbackAndRetry(t *testing.T) {
	public := t.TempDir()
	at := int64(1793404800)
	g := testGroup()
	fail := func(p string, b []byte) error {
		if strings.HasSuffix(p, ".js") {
			return errors.New("injected failure")
		}
		return AppendLine(p, b)
	}
	if e := appendGroup(public, g, at, []any{at, 1}, fail); e == nil {
		t.Fatal("expected failure")
	}
	if e := AppendGroup(public, g, at+60, []any{at + 60, 2}); e != nil {
		t.Fatal(e)
	}
	for _, ext := range []string{".jsonl", ".js"} {
		b, e := os.ReadFile(filepath.Join(public, "data/1m/2026-10-31", "cpu"+ext))
		if e != nil || bytes.Count(b, []byte{'\n'}) != 2 {
			t.Fatal(string(b), e)
		}
	}
}

func TestRepairTailBlockBoundary(t *testing.T) {
	for _, tc := range []struct{ name, input, want string }{
		{"newline at block end", strings.Repeat("x", 4095) + "\n", strings.Repeat("x", 4095) + "\n"},
		{"partial exactly one block", "ok\n" + strings.Repeat("x", 4096), "ok\n"},
		{"partial crosses block", "ok\n" + strings.Repeat("x", 4097), "ok\n"},
		{"newline at block start", "a\n" + strings.Repeat("x", 4095), "a\n"},
		{"no newline exactly one block", strings.Repeat("x", 4096), ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "tail")
			os.WriteFile(p, []byte(tc.input), 0644)
			if e := RepairTail(p); e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(p)
			if e != nil || string(b) != tc.want {
				t.Fatal(len(b), e)
			}
		})
	}
}

func TestAtomicWriteUsesPrivateTemp(t *testing.T) {
	base := t.TempDir()
	public := filepath.Join(base, "public")
	os.Mkdir(public, 0755)
	// The failed rename exposes its source path, without relying on polling timings.
	target := filepath.Join(public, "missing/index.json")
	e := AtomicWrite(target, []byte("{}"), 0644, base)
	var link *os.LinkError
	if !errors.As(e, &link) || filepath.Dir(link.Old) != base {
		t.Fatal(e)
	}
	entries, e := os.ReadDir(base)
	if e != nil || len(entries) != 1 || entries[0].Name() != "public" {
		t.Fatal(entries, e)
	}
	target = filepath.Join(public, "index.json")
	if e = AtomicWrite(target, []byte("{}"), 0644, base); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(target)
	if e != nil || string(b) != "{}" {
		t.Fatal(string(b), e)
	}
}
