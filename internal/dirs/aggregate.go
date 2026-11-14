// Package dirs measures daily directory usage independently of collection.
package dirs

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const depth, top = 2, 20

type Entry struct {
	Path  string `json:"path"`
	Depth int    `json:"depth"`
	Bytes uint64 `json:"bytes"`
}

type entry struct {
	path            string
	bytes           uint64
	dev, ino, nlink uint64 // dev is used only by walk to exclude other filesystems.
	dir             bool
}
type Accumulator struct {
	mount string
	total uint64
	dirs  map[string]uint64
	links map[uint64]struct{}
}

type Result struct {
	Total   uint64
	Entries []Entry
}

func newAccumulator(mount string) *Accumulator {
	return &Accumulator{mount: mount, dirs: map[string]uint64{}, links: map[uint64]struct{}{}}
}

// Add mutates this filesystem's totals; only shallow directories and linked inodes are retained.
// The caller must pass entries from the same device: ino is unique within one filesystem.
func (a *Accumulator) Add(e entry) error {
	if e.path == "" || filepath.IsAbs(e.path) || filepath.Clean(e.path) != e.path || e.path == ".." || strings.HasPrefix(e.path, ".."+string(filepath.Separator)) {
		return fmt.Errorf("invalid usage entry")
	}
	if !e.dir && e.nlink > 1 {
		if _, seen := a.links[e.ino]; seen {
			return nil
		}
		a.links[e.ino] = struct{}{}
	}
	a.total += e.bytes
	parts := strings.Split(e.path, string(filepath.Separator))
	// A file has no own entry; only its directory ancestors receive its blocks.
	n := len(parts)
	if !e.dir {
		n--
	}
	if n > depth {
		n = depth
	}
	if e.path == "." || n == 0 {
		return nil
	}
	for i := 1; i <= n; i++ {
		p := filepath.Join(parts[:i]...)
		a.dirs[p] += e.bytes
	}
	return nil
}

// Finish returns an independent sorted result without changing the accumulator.
func (a *Accumulator) Finish(top int) Result {
	entries := []Entry{}
	for p, b := range a.dirs {
		entries = append(entries, Entry{escapePath(filepath.Join(a.mount, p)), strings.Count(p, string(filepath.Separator)) + 1, b})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Bytes != entries[j].Bytes {
			return entries[i].Bytes > entries[j].Bytes
		}
		return entries[i].Path < entries[j].Path
	})
	if len(entries) > top {
		entries = entries[:top]
	}
	return Result{a.total, entries}
}
func escapePath(path string) string {
	var out strings.Builder
	for len(path) > 0 {
		r, n := utf8.DecodeRuneInString(path)
		if r != '\\' && !(r == utf8.RuneError && n == 1) && unicode.IsPrint(r) {
			out.WriteString(path[:n])
		} else {
			for _, b := range []byte(path[:n]) {
				fmt.Fprintf(&out, `\x%02x`, b)
			}
		}
		path = path[n:]
	}
	return out.String()
}
