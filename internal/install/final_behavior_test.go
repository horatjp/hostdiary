package install

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"hostdiary/internal/collect"
)

func TestUninstallLeavesNonregularDestinations(t *testing.T) {
	for _, path := range []string{binaryPath, servicePath, timerPath} {
		for _, kind := range []string{"symlink", "mask", "directory", "fifo"} {
			t.Run(filepath.Base(path)+"/"+kind, func(t *testing.T) {
				f := newFixture(t)
				f.active = "inactive"
				full := f.path(path)
				var err error
				switch kind {
				case "symlink":
					target := f.path("/source/kept-target")
					writeTestFile(t, target, "kept", 0600)
					err = os.Symlink(target, full)
				case "mask":
					err = os.Symlink("/dev/null", full)
				case "directory":
					err = os.Mkdir(full, 0755)
				case "fifo":
					err = syscall.Mkfifo(full, 0600)
				}
				if err != nil {
					t.Fatal(err)
				}
				before, _ := os.Lstat(full)
				if code := Uninstall(f.env); code != 1 {
					t.Error("expected refusal", code)
				}
				after, err := os.Lstat(full)
				if err != nil {
					t.Fatal("destination removed", err)
				}
				if !os.SameFile(before, after) || before.Mode() != after.Mode() {
					t.Fatal("destination changed")
				}
				if !strings.Contains(f.stderr.String(), path+": not a regular file; left in place") {
					t.Fatal(f.stderr.String())
				}
			})
		}
	}
}

func TestInstallerVersionAndInstalledExecutableWording(t *testing.T) {
	f := newFixture(t)
	if code := Install(f.env); code != 0 {
		t.Fatal(code)
	}
	f.out.Reset()
	f.fail = "start hostdiary-collect.service"
	if code := Install(f.env); code != 1 {
		t.Fatal(code)
	}
	for _, s := range []string{"installer version: " + collect.Version, "runs the installed executable (version " + collect.Version + ")"} {
		if !strings.Contains(f.out.String(), s) {
			t.Fatal(f.out.String())
		}
	}
	if strings.Contains(f.out.String(), "newly installed") {
		t.Fatal(f.out.String())
	}
}

func TestUninstallQueriesOnlyActive(t *testing.T) {
	f := newFixture(t)
	f.active = "inactive"
	if code := Uninstall(f.env); code != 0 {
		t.Fatal(code)
	}
	for _, call := range f.calls {
		if strings.HasPrefix(call, "is-enabled ") {
			t.Fatal("unnecessary enabled query", f.calls)
		}
	}
	if !strings.Contains(f.out.String(), "timer: active=inactive") || strings.Contains(f.out.String(), "enabled=") {
		t.Fatal(f.out.String())
	}
}
