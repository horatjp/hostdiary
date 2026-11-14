package install

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDirectoryWarningsNameGroupAndActualWriters(t *testing.T) {
	for _, tc := range []struct {
		name    string
		mode    os.FileMode
		writers string
	}{
		{"group", 0775, "group '"}, {"other", 0757, "other users"},
		{"both", 0777, "and other users"}, {"neither", 0755, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			if err := os.Chmod(f.path("/usr/local/bin"), tc.mode); err != nil {
				t.Fatal(err)
			}
			warnWritableDirectory(f.env)
			if tc.writers == "" {
				if f.stderr.Len() != 0 {
					t.Fatal(f.stderr.String())
				}
				return
			}
			for _, s := range []string{"group '", tc.writers, "could replace the executable that runs as root"} {
				if !strings.Contains(f.stderr.String(), s) {
					t.Fatal(f.stderr.String())
				}
			}
		})
	}
}

func TestFilePlacementFailureNeverStartsService(t *testing.T) {
	for _, target := range []string{binaryPath, servicePath, timerPath, "/etc/hostdiary"} {
		t.Run(target, func(t *testing.T) {
			f := newFixture(t)
			chown := f.env.Chown
			calls := 0
			f.env.Chown = func(opened *os.File, uid, gid int) error {
				calls++
				failAt := map[string]int{binaryPath: 1, servicePath: 2, timerPath: 3, "/etc/hostdiary": 6}[target]
				if calls == failAt {
					return errors.New("injected ownership failure")
				}
				return chown(opened, uid, gid)
			}
			if code := Install(f.env); code != 1 {
				t.Fatal(code)
			}
			if !reflect.DeepEqual(f.calls, []string{"is-enabled hostdiary-collect.timer", "is-active hostdiary-collect.timer", "is-enabled hostdiary-dirs.timer", "is-active hostdiary-dirs.timer"}) {
				t.Fatal(f.calls)
			}
			if !strings.Contains(f.stderr.String(), "injected ownership failure") || !strings.Contains(f.out.String(), "timer: enabled=enabled, active=active") {
				t.Fatal(f.out.String(), f.stderr.String())
			}
			if target == binaryPath && strings.Contains(f.out.String(), "executable (version") {
				t.Fatal("claimed an executable that was not installed")
			}
			if target == binaryPath && !strings.Contains(f.out.String(), "the timer is still active and runs the installed executable.") {
				t.Fatal("missing active-timer warning after placement failure", f.out.String())
			}
			for path := range snapshot(t, f.env.Root) {
				if strings.HasPrefix(filepath.Base(path), ".hostdiary-") {
					t.Fatal("temporary file leaked", path)
				}
			}
		})
	}
}

func TestEmptyTimerQueryIsUnknownAndFails(t *testing.T) {
	f := newFixture(t)
	run := f.env.Run
	f.env.Run = func(name string, args ...string) ([]byte, []byte, int, error) {
		if args[0] == "is-active" {
			f.calls = append(f.calls, strings.Join(args, " "))
			return nil, nil, 0, nil
		}
		return run(name, args...)
	}
	if code := Install(f.env); code != 1 {
		t.Fatal(code)
	}
	if !strings.Contains(f.out.String(), "timer: enabled=enabled, active=unknown") {
		t.Fatal(f.out.String())
	}
}
