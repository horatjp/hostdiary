package install

import (
	"errors"
	"os"
	"testing"
)

// Replacing the pathname after opening must not redirect the ownership hook
// toward a different inode. All paths and operations stay in the temporary root.
func TestChownReceivesOpenInodeDespitePathReplacement(t *testing.T) {
	for _, kind := range []string{"file", "config-directory"} {
		t.Run(kind, func(t *testing.T) {
			f := newFixture(t)
			victim := f.path("/source/victim")
			if kind == "file" {
				writeTestFile(t, victim, "private", 0600)
			} else if err := os.Mkdir(victim, 0700); err != nil {
				t.Fatal(err)
			}
			before, _ := os.Stat(victim)
			sentinel := errors.New("stop after fd inspection")
			called := false
			f.env.Chown = func(opened *os.File, uid, gid int) error {
				called = true
				original, err := opened.Stat()
				if err != nil {
					t.Fatal(err)
				}
				moved := opened.Name() + "-moved"
				if err := os.Rename(opened.Name(), moved); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(victim, opened.Name()); err != nil {
					t.Fatal(err)
				}
				fdInfo, err := opened.Stat()
				if err != nil {
					t.Fatal(err)
				}
				pathInfo, err := os.Stat(opened.Name())
				if err != nil {
					t.Fatal(err)
				}
				if uid != 0 || gid != 0 || !os.SameFile(original, fdInfo) || os.SameFile(fdInfo, pathInfo) {
					t.Fatal("ownership redirected from opened inode")
				}
				return sentinel
			}
			var err error
			if kind == "file" {
				_, err = placeFile(f.env, f.path(binaryPath), []byte("binary"), 0755)
			} else {
				err = createConfigDirectory(f.env)
			}
			if !called || !errors.Is(err, sentinel) {
				t.Fatal(called, err)
			}
			after, err := os.Stat(victim)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
				t.Fatal("victim metadata changed", err)
			}
			if kind == "file" {
				data, _ := os.ReadFile(victim)
				if string(data) != "private" {
					t.Fatal("victim content changed")
				}
			}
		})
	}
}
