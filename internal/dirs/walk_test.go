package dirs

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// walk wraps the shared logger only for tests of one isolated filesystem.
func walk(root, mount string, lstat func(string) (os.FileInfo, error), stderr io.Writer) Filesystem {
	log := walkErrors{stderr: stderr}
	defer log.summary()
	return walkWithLog(root, mount, lstat, &log)
}

func writeFixture(t *testing.T, root, path string, b []byte) string {
	t.Helper()
	p := filepath.Join(root, path)
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b, 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestWalkTreeBlocksLinksAndSymlink(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "a/b/c/file", bytes.Repeat([]byte("x"), 8192))
	writeFixture(t, root, "direct", []byte("root"))
	if err := os.Link(filepath.Join(root, "a/b/c/file"), filepath.Join(root, "a/link")); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	writeFixture(t, outside, "large", bytes.Repeat([]byte("x"), 65536))
	if err := os.Symlink(outside, filepath.Join(root, "a/symlink")); err != nil {
		t.Fatal(err)
	}
	var want int64
	seen := map[uint64]bool{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		fi, e := os.Lstat(p)
		if e != nil {
			return e
		}
		s := fi.Sys().(*syscall.Stat_t)
		if fi.Mode().IsRegular() && s.Nlink > 1 {
			if seen[s.Ino] {
				return nil
			}
			seen[s.Ino] = true
		}
		want += s.Blocks * 512
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	got := walk(root, "/fixture", os.Lstat, &bytes.Buffer{})
	if !got.Complete || got.Errors != 0 || got.Total != uint64(want) || len(got.Entries) != 2 {
		t.Fatal(got, want)
	}
	if got.Entries[0].Bytes < got.Entries[1].Bytes {
		t.Fatal(got)
	}
}

func TestWalkSkipsOtherDevice(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "other/sub/file", []byte("hidden"))
	stat := func(p string) (os.FileInfo, error) {
		fi, e := os.Lstat(p)
		if e != nil {
			return nil, e
		}
		if p == filepath.Join(root, "other") {
			s := *fi.Sys().(*syscall.Stat_t)
			s.Dev++
			return fakeInfo{fi, &s}, nil
		}
		return fi, nil
	}
	got := walk(root, "/fixture", stat, &bytes.Buffer{})
	fi, _ := os.Lstat(root)
	if got.Total != uint64(fi.Sys().(*syscall.Stat_t).Blocks)*512 || len(got.Entries) != 0 || !got.Complete {
		t.Fatal(got)
	}
}

type fakeInfo struct {
	os.FileInfo
	stat *syscall.Stat_t
}

func (f fakeInfo) Sys() any { return f.stat }

func TestWalkErrorClassification(t *testing.T) {
	for _, tc := range []struct {
		err      error
		complete bool
	}{{fs.ErrNotExist, true}, {fs.ErrPermission, false}} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			root := t.TempDir()
			writeFixture(t, root, "gone", []byte("x"))
			stat := func(p string) (os.FileInfo, error) {
				if filepath.Base(p) == "gone" {
					return nil, tc.err
				}
				return os.Lstat(p)
			}
			var log bytes.Buffer
			got := walk(root, "/fixture", stat, &log)
			if got.Errors != 1 || got.Complete != tc.complete || log.Len() == 0 {
				t.Fatal(got, log.String())
			}
		})
	}
}

func TestUnreadableDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod does not deny root")
	}
	root := t.TempDir()
	p := filepath.Join(root, "blocked")
	if err := os.Mkdir(p, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(p, 0700)
	got := walk(root, "/fixture", os.Lstat, &bytes.Buffer{})
	if got.Complete || got.Errors != 1 {
		t.Fatal(got)
	}
}

func TestWalkMissingMount(t *testing.T) {
	got := walk(filepath.Join(t.TempDir(), "missing"), "/fixture", os.Lstat, &bytes.Buffer{})
	// A missing mount is a failed measurement, not a disappearing child entry.
	if got.Complete || got.Errors != 1 {
		t.Fatal(got)
	}
}

func TestWalkUnexpectedStat(t *testing.T) {
	root := t.TempDir()
	got := walk(root, "/fixture", func(string) (os.FileInfo, error) { return nil, errors.New("failed") }, &bytes.Buffer{})
	if got.Complete || got.Errors != 1 {
		t.Fatal(got)
	}
}

func TestSparseFileUsesAllocatedBlocks(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "sparse")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err = f.Truncate(64 << 20); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(p)
	if err != nil {
		t.Fatal(err)
	}
	ri, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	want := (fi.Sys().(*syscall.Stat_t).Blocks + ri.Sys().(*syscall.Stat_t).Blocks) * 512
	got := walk(root, "/fixture", os.Lstat, &bytes.Buffer{})
	if !got.Complete || got.Total != uint64(want) || len(got.Entries) != 0 {
		t.Fatal(got, want)
	}
}
