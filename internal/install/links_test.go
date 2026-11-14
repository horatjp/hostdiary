package install

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// A rejected destination must preserve both the link and its target metadata.
func TestPlaceRejectsSymlinks(t *testing.T) {
	for _, kind := range []string{"absolute", "relative", "dangling"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			path := f.path(binaryPath)
			target := filepath.Join(filepath.Dir(path), "target")
			writeTestFile(t, target, "same", 0600)
			before, _ := os.Stat(target)
			link := target
			if kind == "relative" {
				link = "target"
			}
			if kind == "dangling" {
				link = target + "-missing"
			}
			if err := os.Symlink(link, path); err != nil {
				t.Fatal(err)
			}
			f.env.Chown = func(*os.File, int, int) error { t.Error("chown on rejected symlink"); return nil }
			if _, err := placeFile(f.env, path, []byte("same"), 0755); err == nil {
				t.Fatal("accepted symlink")
			}
			if got, err := os.Readlink(path); err != nil || got != link {
				t.Fatal(got, err)
			}
			after, _ := os.Stat(target)
			if !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("target changed")
			}
		})
	}
}

// Metadata repair must detach the installation from old hardlinks and open fds.
func TestMetadataRepairDetachesOldInode(t *testing.T) {
	for _, mismatch := range []string{"mode", "owner"} {
		t.Run(mismatch, func(t *testing.T) {
			f := newFixture(t)
			path := f.path(binaryPath)
			mode := os.FileMode(0755)
			if mismatch == "mode" {
				mode = 0666
			}
			writeTestFile(t, path, "same", mode)
			if mismatch == "owner" {
				f.setOwner(t, binaryPath, 123, 456)
			}
			before, _ := os.Stat(path)
			link := f.path("/source/old-link")
			if err := os.Link(path, link); err != nil {
				t.Fatal(err)
			}
			fd, err := os.OpenFile(path, os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer fd.Close()
			status, err := placeFile(f.env, path, []byte("same"), 0755)
			if err != nil || status != "fixed ownership/mode" {
				t.Fatal(status, err)
			}
			after, _ := os.Stat(path)
			if os.SameFile(before, after) {
				t.Error("metadata repair reused old inode")
			}
			if err := os.WriteFile(link, []byte("evil"), mode); err != nil {
				t.Fatal(err)
			}
			if _, err := fd.WriteAt([]byte("EVIL"), 0); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != "same" {
				t.Fatalf("old inode modified installed file: %q %v", got, err)
			}
			old, _ := os.Stat(link)
			if old.Mode() != before.Mode() {
				t.Fatal("old inode metadata changed")
			}
		})
	}
}

func TestInstallRejectsMaskedUnits(t *testing.T) {
	for _, path := range []string{servicePath, timerPath} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			f := newFixture(t)
			if err := os.Symlink("/dev/null", f.path(path)); err != nil {
				t.Fatal(err)
			}
			if code := Install(f.env); code != 1 {
				t.Fatal(code)
			}
			want := "masked; run systemctl unmask " + filepath.Base(path) + " first"
			if !strings.Contains(f.stderr.String(), want) {
				t.Fatal(f.stderr.String())
			}
			if got, err := os.Readlink(f.path(path)); err != nil || got != "/dev/null" {
				t.Fatal(got, err)
			}
			for _, call := range f.calls {
				if strings.HasPrefix(call, "start ") || strings.HasPrefix(call, "enable ") {
					t.Fatal(f.calls)
				}
			}
		})
	}
}

func TestPlaceRejectsDirectoryAndFIFO(t *testing.T) {
	for _, kind := range []string{"directory", "fifo"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			path := f.path(binaryPath)
			var err error
			if kind == "directory" {
				err = os.Mkdir(path, 0755)
			} else {
				err = syscall.Mkfifo(path, 0600)
			}
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.Lstat(path)
			f.env.Chown = func(*os.File, int, int) error { t.Error("chown on nonregular file"); return nil }
			if _, err := placeFile(f.env, path, []byte("binary"), 0755); err == nil {
				t.Fatal("accepted nonregular file")
			}
			after, _ := os.Lstat(path)
			if !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("nonregular destination changed")
			}
		})
	}
}
