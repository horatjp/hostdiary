package dirs

import (
	"fmt"
	"reflect"
	"testing"
)

// Contract: direct files affect total only; ancestors overlap; non-directory links count once.
func TestAggregateContract(t *testing.T) {
	a := newAccumulator("/")
	for _, e := range []entry{
		{path: ".", bytes: 1, dir: true}, {path: "root-file", bytes: 100},
		{path: "a", bytes: 2, dir: true, nlink: 3}, {path: "a/b", bytes: 3, dir: true, nlink: 3},
		{path: "a/b/c", bytes: 4, dir: true}, {path: "a/b/c/file", bytes: 10, nlink: 2, ino: 9},
		{path: "a/link", bytes: 10, nlink: 2, ino: 9},
	} {
		if err := a.Add(e); err != nil {
			t.Fatal(err)
		}
	}
	got := a.Finish(20)
	want := []Entry{{Path: "/a", Depth: 1, Bytes: 19}, {Path: "/a/b", Depth: 2, Bytes: 17}}
	if got.Total != 120 || !reflect.DeepEqual(got.Entries, want) {
		t.Fatal(got)
	}
	// Finish is repeatable, and its output is detached from subsequent Add calls.
	if !reflect.DeepEqual(a.Finish(20), got) {
		t.Fatal("Finish mutated input")
	}
	if err := a.Add(entry{path: "a/new", bytes: 7}); err != nil {
		t.Fatal(err)
	}
	if got.Total != 120 || got.Entries[0].Bytes != 19 || a.Finish(20).Total != 127 {
		t.Fatal(got, a.Finish(20))
	}
}

func TestTopAndEmpty(t *testing.T) {
	a := newAccumulator("/mnt")
	if got := a.Finish(20); got.Total != 0 || got.Entries == nil || len(got.Entries) != 0 {
		t.Fatal(got)
	}
	for i := 24; i >= 0; i-- {
		if err := a.Add(entry{path: fmt.Sprintf("d%02d", i), dir: true, bytes: 5}); err != nil {
			t.Fatal(err)
		}
	}
	got := a.Finish(20)
	if len(got.Entries) != 20 || got.Entries[0].Path != "/mnt/d00" || got.Entries[19].Path != "/mnt/d19" {
		t.Fatal(got)
	}
	if got := a.Finish(0); len(got.Entries) != 0 {
		t.Fatal(got)
	}
}

func TestAddRejectsOutside(t *testing.T) {
	a := newAccumulator("/")
	for _, e := range []entry{{path: "../x"}, {path: "/x"}, {path: ""}, {path: "a/../b"}} {
		if err := a.Add(e); err == nil {
			t.Fatal(e)
		}
	}
	if got := a.Finish(20); got.Total != 0 || len(got.Entries) != 0 {
		t.Fatal(got)
	}
}

func TestEscapePath(t *testing.T) {
	for _, tc := range [][2]string{{"/日本語/ok", "/日本語/ok"}, {"a\\b\n\x00\xff", `a\x5cb\x0a\x00\xff`}, {"\u0085", `\xc2\x85`}, {"\\xff", `\x5cxff`}} {
		if got := escapePath(tc[0]); got != tc[1] {
			t.Fatalf("%q != %q", got, tc[1])
		}
	}
}

func TestAccumulatorRetainsOnlyShallowDirectoriesAndLinkedInodes(t *testing.T) {
	a := newAccumulator("/")
	for i := 0; i < 2000; i++ {
		if err := a.Add(entry{path: fmt.Sprintf("a/b/c/file%04d", i), bytes: 1, ino: uint64(i), nlink: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if len(a.dirs) != 2 || len(a.links) != 0 || a.Finish(20).Total != 2000 {
		t.Fatal(a)
	}
	for i := 0; i < 2; i++ {
		if err := a.Add(entry{path: fmt.Sprintf("a/b/link%d", i), bytes: 512, ino: 3000, nlink: 2}); err != nil {
			t.Fatal(err)
		}
	}
	if len(a.links) != 1 || a.Finish(20).Total != 2512 {
		t.Fatal(a)
	}
}
