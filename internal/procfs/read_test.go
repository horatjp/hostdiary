package procfs

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"hostdiary/internal/config"
	"hostdiary/internal/store"
)

func snapshotRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if e := filepath.WalkDir("../../testdata", func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel("../../testdata", p)
		if e != nil {
			return e
		}
		dest := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(dest, b, 0644)
	}); e != nil {
		t.Fatal(e)
	}
	return root
}

func TestVirtualDiskIODefaultExclusions(t *testing.T) {
	root := snapshotRoot(t)
	f, err := os.OpenFile(filepath.Join(root, "proc/diskstats"), os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString("\n43 0 nbd0 1 0 2 0 1 0 2 0 0 3 0\n252 0 zram0 1 0 2 0 1 0 2 0 0 3 0\n")
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"nbd0", "zram0"} {
		if err := os.MkdirAll(filepath.Join(root, "sys/block", name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	for _, include := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "include"}[include], func(t *testing.T) {
			c := config.Config{Disk: config.Filter{Exclude: []string{"/", "/space disk"}}}
			if include {
				c.DiskIO.Include = []string{"nbd0", "zram0"}
			}
			s := ReadSnapshot(root, c)
			if len(s.Errors) != 0 || len(s.Unavailable) != 0 {
				t.Fatal(s.Errors, s.Unavailable)
			}
			for _, name := range []string{"nbd0", "zram0"} {
				_, present := s.DiskIO[name]
				if present != include {
					t.Fatal(name, s.DiskIO)
				}
			}
			want := 1
			if include {
				want = 2
			}
			if len(s.DiskIO) != want {
				t.Fatal(s.DiskIO)
			}
		})
	}
}
func TestSnapshotSelectionAndMissingIncludes(t *testing.T) {
	root := snapshotRoot(t)
	c := config.Config{Net: config.Filter{Include: []string{"lo", "missing"}}, DiskIO: config.Filter{Include: []string{"loop0", "sda1"}}, Disk: config.Filter{Exclude: []string{"/", "/space disk"}}}
	s := ReadSnapshot(root, c)
	if len(s.Errors) != 0 || len(s.Net) != 1 || s.Net["lo"][0] != 100 || len(s.DiskIO) != 1 || s.DiskIO["loop0"][0] != 2 {
		t.Fatalf("%+v", s)
	}
	for _, id := range []string{store.GroupID("net", "missing"), store.GroupID("diskio", "sda1")} {
		found := false
		for _, u := range s.Unavailable {
			found = found || u.Group == id && u.Reason == "not_found"
		}
		if !found {
			t.Fatal(id, s.Unavailable)
		}
	}
}
func TestSnapshotDefaultSelectionAndVanishedPID(t *testing.T) {
	root := snapshotRoot(t)
	os.MkdirAll(filepath.Join(root, "proc/104"), 0755)
	s := ReadSnapshot(root, config.Config{Disk: config.Filter{Exclude: []string{"/", "/space disk"}}})
	if !s.ProcessesValid || len(s.Processes) != 3 || len(s.Net) != 1 || len(s.DiskIO) != 1 {
		t.Fatalf("%+v", s)
	}
	if _, ok := s.Net["eth0"]; !ok {
		t.Fatal(s.Net)
	}
	if _, ok := s.DiskIO["sda"]; !ok {
		t.Fatal(s.DiskIO)
	}
}
func TestUnreadablePIDDoesNotPublishPartialTotals(t *testing.T) {
	root := snapshotRoot(t)
	os.WriteFile(filepath.Join(root, "proc/101/stat"), []byte("invalid"), 0644)
	s := ReadSnapshot(root, config.Config{Disk: config.Filter{Exclude: []string{"/", "/space disk"}}})
	if s.ProcessesValid || len(s.Errors) != 1 {
		t.Fatalf("%+v", s)
	}
}
