package install

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestInstallUninstallPreservesPrivateFiles(t *testing.T) {
	f := newFixture(t)
	kept := map[string]string{
		"/etc/hostdiary/config.json":                                 "invalid config: must not be read",
		"/var/lib/hostdiary/state.json":                              "private state",
		"/var/lib/hostdiary/public/data/history":                     "history",
		"/etc/systemd/system/hostdiary-collect.service.d/local.conf": "local service override",
		"/etc/systemd/system/hostdiary-collect.timer.d/local.conf":   "local timer override",
	}
	before := map[string]os.FileInfo{}
	for path, content := range kept {
		writeTestFile(t, f.path(path), content, 0600)
		before[path], _ = os.Stat(f.path(path))
	}
	if err := os.Chmod(f.path("/etc/hostdiary"), 0700); err != nil {
		t.Fatal(err)
	}
	if code := Install(f.env); code != 0 {
		t.Fatal(code)
	}
	fi, _ := os.Stat(f.path("/etc/hostdiary"))
	if fi.Mode().Perm() != 0700 {
		t.Fatal("existing directory changed")
	}
	wants := "/etc/systemd/system/timers.target.wants/hostdiary-collect.timer"
	if err := os.MkdirAll(filepath.Dir(f.path(wants)), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../hostdiary-collect.timer", f.path(wants)); err != nil {
		t.Fatal(err)
	}
	f.calls = nil
	f.out.Reset()
	f.enabled, f.active = "disabled", "inactive"
	run := f.env.Run
	f.env.Run = func(name string, args ...string) ([]byte, []byte, int, error) {
		_, err := os.Stat(f.path(timerPath))
		if args[0] == "disable" && err != nil {
			t.Fatal("deleted before disable")
		}
		if args[0] == "daemon-reload" && !os.IsNotExist(err) {
			t.Fatal("reload before deletion")
		}
		return run(name, args...)
	}
	if code := Uninstall(f.env); code != 0 {
		t.Fatal(code, f.stderr.String())
	}
	if !reflect.DeepEqual(f.calls, []string{"disable --now hostdiary-collect.timer", "disable --now hostdiary-dirs.timer", "daemon-reload", "is-active hostdiary-collect.timer", "is-active hostdiary-dirs.timer", "is-active hostdiary-dirs.service"}) {
		t.Fatal(f.calls)
	}
	for _, path := range []string{binaryPath, servicePath, timerPath, dirsServicePath, dirsTimerPath} {
		if _, err := os.Lstat(f.path(path)); !os.IsNotExist(err) {
			t.Fatal(path, err)
		}
	}
	for path, content := range kept {
		got, err := os.ReadFile(f.path(path))
		if err != nil || string(got) != content {
			t.Fatal(path, err)
		}
		fi, _ := os.Stat(f.path(path))
		if !os.SameFile(before[path], fi) || !before[path].ModTime().Equal(fi.ModTime()) || before[path].Mode() != fi.Mode() {
			t.Fatal("preserved file changed", path)
		}
	}
	for _, s := range []string{"sudo rm -rf /var/lib/hostdiary /etc/hostdiary", wants, "hostdiary-collect.service.d", "hostdiary-collect.timer.d"} {
		if !strings.Contains(f.out.String(), s) {
			t.Fatal(f.out.String())
		}
	}
}

func TestUninstallEmptyAndFailures(t *testing.T) {
	for _, name := range []string{"empty", "disable", "reload", "delete"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			if name != "empty" {
				if code := Install(f.env); code != 0 {
					t.Fatal(code)
				}
			}
			f.active = "inactive"
			switch name {
			case "disable":
				f.fail = "disable --now hostdiary-collect.timer"
			case "reload":
				f.fail = "daemon-reload"
			case "delete":
				if err := os.Remove(f.path(servicePath)); err != nil {
					t.Fatal(err)
				}
				writeTestFile(t, filepath.Join(f.path(servicePath), "keep"), "blocks deletion", 0644)
			}
			f.calls = nil
			f.out.Reset()
			want := 1
			if name == "empty" {
				want = 0
			}
			if code := Uninstall(f.env); code != want {
				t.Fatal(code, f.stderr.String())
			}
			wantCalls := []string{"daemon-reload", "is-active hostdiary-collect.timer", "is-active hostdiary-dirs.timer", "is-active hostdiary-dirs.service"}
			if name != "empty" {
				wantCalls = append([]string{"disable --now hostdiary-collect.timer", "disable --now hostdiary-dirs.timer"}, wantCalls...)
			}
			if !reflect.DeepEqual(wantCalls, f.calls) {
				t.Fatal(f.calls)
			}
			for _, path := range []string{binaryPath, timerPath} {
				if _, err := os.Lstat(f.path(path)); !os.IsNotExist(err) {
					t.Fatal(path, err)
				}
			}
			if !strings.Contains(f.out.String(), "sudo rm -rf /var/lib/hostdiary /etc/hostdiary") {
				t.Fatal(f.out.String())
			}
			if name == "empty" && strings.Count(f.out.String(), ": absent") != 5 {
				t.Fatal(f.out.String())
			}
			if name == "disable" || name == "reload" {
				if !strings.Contains(f.stderr.String(), "injected failure") {
					t.Fatal(f.stderr.String())
				}
			}
		})
	}
}
