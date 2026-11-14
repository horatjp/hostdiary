package install

import (
	"hostdiary/packaging/systemd"
	"os"
	"strings"
	"testing"
)

func TestDirsInstallUnitsAndTimerSuccessRequired(t *testing.T) {
	for _, state := range []string{"active", "inactive"} {
		t.Run(state, func(t *testing.T) {
			f := newFixture(t)
			run := f.env.Run
			f.env.Run = func(name string, args ...string) ([]byte, []byte, int, error) {
				if args[0] == "is-active" && args[1] == "hostdiary-dirs.timer" && state == "inactive" {
					f.calls = append(f.calls, strings.Join(args, " "))
					return []byte("inactive\n"), nil, 3, nil
				}
				return run(name, args...)
			}
			want := 0
			if state == "inactive" {
				want = 1
			}
			if code := Install(f.env); code != want {
				t.Fatal(code, f.out.String())
			}
			for path, content := range map[string]string{"/etc/systemd/system/hostdiary-dirs.service": systemd.DirsService, "/etc/systemd/system/hostdiary-dirs.timer": systemd.DirsTimer} {
				b, err := os.ReadFile(f.path(path))
				if err != nil || string(b) != content {
					t.Fatal(path, err)
				}
			}
			calls := strings.Join(f.calls, "\n")
			if !strings.Contains(calls, "enable --now hostdiary-collect.timer\nenable --now hostdiary-dirs.timer") || strings.Contains(calls, "start hostdiary-dirs.service") {
				t.Fatal(calls)
			}
			if !strings.Contains(f.out.String(), "dirs timer: enabled=enabled, active="+state) || !strings.Contains(f.out.String(), "systemctl start hostdiary-dirs.service") {
				t.Fatal(f.out.String())
			}
		})
	}
}

func TestDirsUninstallReportsRunningServiceWithoutFailure(t *testing.T) {
	for _, state := range []string{"activating", "active", "deactivating", "inactive"} {
		t.Run(state, func(t *testing.T) {
			f := newFixture(t)
			if code := Install(f.env); code != 0 {
				t.Fatal(code)
			}
			f.active = "inactive"
			run := f.env.Run
			f.env.Run = func(name string, args ...string) ([]byte, []byte, int, error) {
				if args[0] == "is-active" && args[1] == "hostdiary-dirs.service" {
					f.calls = append(f.calls, strings.Join(args, " "))
					code := 3
					if state == "active" {
						code = 0
					}
					return []byte(state + "\n"), nil, code, nil
				}
				return run(name, args...)
			}
			f.calls = nil
			f.out.Reset()
			if code := Uninstall(f.env); code != 0 {
				t.Fatal(code, f.stderr.String())
			}
			calls := strings.Join(f.calls, "\n")
			for _, s := range []string{"disable --now hostdiary-dirs.timer", "is-active hostdiary-dirs.timer", "is-active hostdiary-dirs.service"} {
				if !strings.Contains(calls, s) {
					t.Fatal(calls)
				}
			}
			for _, ext := range []string{"service", "timer"} {
				if _, err := os.Lstat(f.path("/etc/systemd/system/hostdiary-dirs." + ext)); !os.IsNotExist(err) {
					t.Fatal(err)
				}
			}
			if !strings.Contains(f.out.String(), "dirs service: active="+state) || strings.Contains(f.out.String(), "systemctl stop hostdiary-dirs.service") != (state != "inactive") {
				t.Fatal(f.out.String())
			}
		})
	}
}

func TestInstallNextListsBothTimers(t *testing.T) {
	f := newFixture(t)
	if code := Install(f.env); code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(f.out.String(), "Next: systemctl list-timers 'hostdiary-*.timer'\n") {
		t.Fatal(f.out.String())
	}
}

func TestDirsTimerFailuresAndProtectedUnits(t *testing.T) {
	for _, call := range []string{"enable --now hostdiary-dirs.timer", "is-enabled hostdiary-dirs.timer", "is-active hostdiary-dirs.timer"} {
		t.Run(call, func(t *testing.T) {
			f := newFixture(t)
			f.fail = call
			if code := Install(f.env); code != 1 {
				t.Fatal(code, f.out.String())
			}
		})
	}
	for _, path := range []string{dirsServicePath, dirsTimerPath} {
		t.Run(path, func(t *testing.T) {
			f := newFixture(t)
			if err := os.Symlink("/dev/null", f.path(path)); err != nil {
				t.Fatal(err)
			}
			if code := Install(f.env); code != 1 {
				t.Fatal(code)
			}
			if target, err := os.Readlink(f.path(path)); err != nil || target != "/dev/null" {
				t.Fatal(target, err)
			}
		})
	}
}

func TestDirsDisableFailureStillRemovesUnits(t *testing.T) {
	f := newFixture(t)
	if code := Install(f.env); code != 0 {
		t.Fatal(code)
	}
	f.active = "inactive"
	f.fail = "disable --now hostdiary-dirs.timer"
	if code := Uninstall(f.env); code != 1 {
		t.Fatal(code)
	}
	for _, path := range []string{dirsServicePath, dirsTimerPath} {
		if _, err := os.Stat(f.path(path)); !os.IsNotExist(err) {
			t.Fatal(path, err)
		}
	}
}
