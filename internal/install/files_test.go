package install

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTestFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

// Equal bytes, ownership and all permission bits preserve the inode and mtime.
// Metadata-only repairs replace the inode; failed replacement preserves the old file.
func TestPlaceFile(t *testing.T) {
	for _, tc := range []struct {
		name, old, content, want string
		mode                     os.FileMode
		owner                    int
	}{
		{"new", "", "new", "installed", 0, 0},
		{"equal", "new", "new", "unchanged", 0755, 0},
		{"different", "old", "new", "replaced (previous content differed; use systemctl edit for local changes)", 0755, 0},
		{"writable", "new", "new", "fixed ownership/mode", 0666, 0},
		{"owner", "new", "new", "fixed ownership/mode", 0755, 123},
		{"setuid", "new", "new", "fixed ownership/mode", 0755 | os.ModeSetuid, 0},
		{"setgid", "new", "new", "fixed ownership/mode", 0755 | os.ModeSetgid, 0},
		{"sticky", "new", "new", "fixed ownership/mode", 0755 | os.ModeSticky, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "hostdiary")
			var before os.FileInfo
			if tc.old != "" {
				writeTestFile(t, path, tc.old, tc.mode)
				stamp := time.Unix(1234567890, 0)
				if err := os.Chtimes(path, stamp, stamp); err != nil {
					t.Fatal(err)
				}
				// Changing timestamps can clear setuid/setgid on the host filesystem.
				if err := os.Chmod(path, tc.mode); err != nil {
					t.Fatal(err)
				}
				before, _ = os.Stat(path)
				if before.Mode() != tc.mode {
					t.Skipf("filesystem did not retain requested mode %v (got %v)", tc.mode, before.Mode())
				}
			}
			owner := tc.owner
			calls := 0
			env := Env{
				Owner: func(os.FileInfo) (int, int) { return owner, owner },
				Chown: func(opened *os.File, uid, gid int) error {
					calls++
					if uid != 0 || gid != 0 {
						t.Fatal(uid, gid)
					}
					if tc.want != "unchanged" {
						fi, err := opened.Stat()
						if err != nil || fi.Mode().Perm() != 0600 {
							t.Fatalf("temporary file before chown: %v %v", fi, err)
						}
					}
					owner = 0
					return nil
				},
			}
			status, err := placeFile(env, path, []byte(tc.content), 0755)
			if err != nil || status != tc.want {
				t.Fatalf("%q %v", status, err)
			}
			got, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(got, []byte(tc.content)) {
				t.Fatalf("content %q %v", got, err)
			}
			after, err := os.Stat(path)
			if err != nil || after.Mode() != 0755 || owner != 0 {
				t.Fatalf("metadata %v %v owner=%d", after, err, owner)
			}
			if tc.want == "unchanged" && (calls != 0 || !os.SameFile(before, after) || !before.ModTime().Equal(after.ModTime())) {
				t.Fatal("equal file changed")
			}
			if tc.want == "fixed ownership/mode" && os.SameFile(before, after) {
				t.Fatal("metadata repair kept old inode")
			}
			if tc.want != "unchanged" && calls != 1 {
				t.Fatalf("chown calls=%d", calls)
			}
		})
	}
}

type modeInfo struct {
	os.FileInfo
	mode os.FileMode
}

func (fi modeInfo) Mode() os.FileMode { return fi.mode }

// All special bits must participate even on test filesystems that strip them.
func TestMetadataMatchesIncludesSpecialBitsAndBothOwners(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     os.FileMode
		uid, gid int
		want     bool
	}{
		{"exact", 0755, 0, 0, true}, {"setuid", 0755 | os.ModeSetuid, 0, 0, false},
		{"setgid", 0755 | os.ModeSetgid, 0, 0, false}, {"sticky", 0755 | os.ModeSticky, 0, 0, false},
		{"writable", 0777, 0, 0, false}, {"uid", 0755, 123, 0, false}, {"gid", 0755, 0, 123, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := Env{Owner: func(os.FileInfo) (int, int) { return tc.uid, tc.gid }}
			if got := metadataMatches(env, modeInfo{mode: tc.mode}, 0755); got != tc.want {
				t.Fatal(got)
			}
		})
	}
}

func TestPlaceFileFailureCleansTemporaryFile(t *testing.T) {
	for _, fail := range []string{"chown", "rename"} {
		t.Run(fail, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "target")
			writeTestFile(t, path, "old", 0644)
			env := Env{Owner: func(os.FileInfo) (int, int) { return 0, 0 }, Chown: func(*os.File, int, int) error {
				if fail == "chown" {
					return errors.New("injected chown failure")
				}
				// Force rename to fail after the temporary file has been created.
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0755); err != nil {
					t.Fatal(err)
				}
				return nil
			}}
			if _, err := placeFile(env, path, []byte("new"), 0644); err == nil {
				t.Fatal("expected failure")
			}
			entries, err := os.ReadDir(dir)
			if err != nil || len(entries) != 1 || entries[0].Name() != "target" {
				t.Fatalf("temporary file leaked: %v %v", entries, err)
			}
			if fail == "chown" {
				got, _ := os.ReadFile(path)
				if string(got) != "old" {
					t.Fatal("old file changed")
				}
			}
		})
	}
}
