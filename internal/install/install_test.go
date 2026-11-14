package install

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"hostdiary/internal/collect"
	"hostdiary/packaging/systemd"
)

type fixture struct {
	env                   Env
	out, stderr           bytes.Buffer
	calls                 []string
	owners                map[uint64][2]int
	fail, enabled, active string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{owners: map[uint64][2]int{}, enabled: "enabled", active: "active"}
	f.env = Env{Root: t.TempDir(), UID: 0, Systemctl: "/fake/systemctl", Stdout: &f.out, Stderr: &f.stderr}
	f.env.Self = f.path("/source/hostdiary")
	for _, dir := range []string{"/run/systemd/system", "/usr/local/bin", "/etc/systemd/system"} {
		if err := os.MkdirAll(f.path(dir), 0755); err != nil {
			t.Fatal(err)
		}
	}
	writeTestFile(t, f.env.Self, "executable bytes", 0755)
	f.env.Chown = func(opened *os.File, uid, gid int) error {
		fi, err := opened.Stat()
		if err != nil {
			return err
		}
		f.owners[fi.Sys().(*syscall.Stat_t).Ino] = [2]int{uid, gid}
		return nil
	}
	f.env.Owner = func(fi os.FileInfo) (int, int) { o := f.owners[fi.Sys().(*syscall.Stat_t).Ino]; return o[0], o[1] }
	f.env.Run = func(name string, args ...string) ([]byte, []byte, int, error) {
		if name != f.env.Systemctl {
			t.Fatalf("unexpected command %q", name)
		}
		call := strings.Join(args, " ")
		f.calls = append(f.calls, call)
		if call == f.fail {
			return nil, []byte("injected failure\n"), 1, errors.New("command failed")
		}
		switch args[0] {
		case "is-enabled":
			if f.enabled != "enabled" {
				return []byte(f.enabled + "\n"), nil, 1, nil
			}
			return []byte(f.enabled + "\n"), nil, 0, nil
		case "is-active":
			if f.active != "active" {
				return []byte(f.active + "\n"), nil, 3, nil
			}
			return []byte(f.active + "\n"), nil, 0, nil
		case "status":
			return []byte("service diagnostics\n"), nil, 3, nil
		case "show":
			return []byte("ExecMainStatus=1\n"), nil, 0, nil
		}
		return nil, nil, 0, nil
	}
	return f
}

func (f *fixture) path(p string) string { return filepath.Join(f.env.Root, p) }

func (f *fixture) setOwner(t *testing.T, path string, uid, gid int) {
	t.Helper()
	fi, err := os.Stat(f.path(path))
	if err != nil {
		t.Fatal(err)
	}
	f.owners[fi.Sys().(*syscall.Stat_t).Ino] = [2]int{uid, gid}
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		value := fi.Mode().String()
		if fi.Mode().IsRegular() {
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += string(b)
		}
		result[path] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestInstallAndRepeat(t *testing.T) {
	f := newFixture(t)
	for pass := 0; pass < 2; pass++ {
		f.out.Reset()
		f.calls = nil
		before := map[string]os.FileInfo{}
		for _, p := range []string{binaryPath, servicePath, timerPath, dirsServicePath, dirsTimerPath} {
			before[p], _ = os.Stat(f.path(p))
		}
		if code := Install(f.env); code != 0 {
			t.Fatalf("code=%d: %s", code, f.stderr.String())
		}
		wantCalls := []string{"daemon-reload", "start hostdiary-collect.service", "enable --now hostdiary-collect.timer", "enable --now hostdiary-dirs.timer", "is-enabled hostdiary-collect.timer", "is-active hostdiary-collect.timer", "is-enabled hostdiary-dirs.timer", "is-active hostdiary-dirs.timer"}
		if !reflect.DeepEqual(f.calls, wantCalls) {
			t.Fatal(f.calls)
		}
		for _, file := range []struct {
			path, content string
			mode          os.FileMode
		}{
			{binaryPath, "executable bytes", 0755}, {servicePath, systemd.Service, 0644}, {timerPath, systemd.Timer, 0644}, {dirsServicePath, systemd.DirsService, 0644}, {dirsTimerPath, systemd.DirsTimer, 0644},
		} {
			b, err := os.ReadFile(f.path(file.path))
			if err != nil || string(b) != file.content {
				t.Fatal(file.path, err)
			}
			fi, _ := os.Stat(f.path(file.path))
			if fi.Mode() != file.mode {
				t.Fatal(fi.Mode())
			}
			if pass == 1 && (!os.SameFile(before[file.path], fi) || !before[file.path].ModTime().Equal(fi.ModTime())) {
				t.Fatal("repeat changed file")
			}
			status := "installed"
			if pass == 1 {
				status = "unchanged"
			}
			if !strings.Contains(f.out.String(), file.path+": "+status) {
				t.Fatal(f.out.String())
			}
		}
		fi, err := os.Stat(f.path("/etc/hostdiary"))
		if err != nil || fi.Mode().Perm() != 0755 {
			t.Fatal(fi, err)
		}
		for _, s := range []string{collect.Version, "timer: enabled=enabled, active=active", "systemctl list-timers", "journalctl", "README"} {
			if !strings.Contains(f.out.String(), s) {
				t.Fatal(f.out.String())
			}
		}
	}
}

func TestPreconditionsHaveNoSideEffects(t *testing.T) {
	for _, change := range []string{"uid", "systemd", "systemd-file", "systemctl", "bin-dir", "unit-dir", "self", "run", "chown", "owner", "root"} {
		t.Run(change, func(t *testing.T) {
			for _, uninstall := range []bool{false, true} {
				f := newFixture(t)
				root := f.env.Root
				switch change {
				case "uid":
					f.env.UID = 123
				case "systemd", "systemd-file":
					if err := os.Remove(f.path("/run/systemd/system")); err != nil {
						t.Fatal(err)
					}
					if change == "systemd-file" {
						writeTestFile(t, f.path("/run/systemd/system"), "x", 0644)
					}
				case "systemctl":
					f.env.Systemctl = ""
				case "bin-dir":
					if uninstall {
						continue
					}
					if err := os.Remove(f.path("/usr/local/bin")); err != nil {
						t.Fatal(err)
					}
				case "unit-dir":
					if uninstall {
						continue
					}
					if err := os.Remove(f.path("/etc/systemd/system")); err != nil {
						t.Fatal(err)
					}
				case "self":
					if uninstall {
						continue
					}
					f.env.Self = f.path("/missing")
				case "run":
					f.env.Run = nil
				case "chown":
					if uninstall {
						continue
					}
					f.env.Chown = nil
				case "owner":
					if uninstall {
						continue
					}
					f.env.Owner = nil
				case "root":
					f.env.Root = "relative"
				}
				before := snapshot(t, root)
				var code int
				if uninstall {
					code = Uninstall(f.env)
				} else {
					code = Install(f.env)
				}
				if code != 2 || len(f.calls) != 0 {
					t.Fatal(code, f.calls)
				}
				if !reflect.DeepEqual(before, snapshot(t, root)) {
					t.Fatal("precondition changed files")
				}
			}
		})
	}
}

func TestInstallFailureStateAndSummary(t *testing.T) {
	for _, tc := range []struct {
		name, fail, enabled, active string
		calls                       []string
	}{
		{"reload", "daemon-reload", "enabled", "active", []string{"daemon-reload"}},
		{"start", "start hostdiary-collect.service", "disabled", "inactive", []string{"daemon-reload", "start hostdiary-collect.service", "status --no-pager -n 30 hostdiary-collect.service", "show -p ExecMainStatus hostdiary-collect.service"}},
		{"start-active", "start hostdiary-collect.service", "enabled", "active", []string{"daemon-reload", "start hostdiary-collect.service", "status --no-pager -n 30 hostdiary-collect.service", "show -p ExecMainStatus hostdiary-collect.service"}},
		{"enable", "enable --now hostdiary-collect.timer", "disabled", "active", []string{"daemon-reload", "start hostdiary-collect.service", "enable --now hostdiary-collect.timer"}},
		{"disabled", "", "disabled", "active", []string{"daemon-reload", "start hostdiary-collect.service", "enable --now hostdiary-collect.timer"}},
		{"inactive", "", "enabled", "inactive", []string{"daemon-reload", "start hostdiary-collect.service", "enable --now hostdiary-collect.timer"}},
		{"query", "is-active hostdiary-collect.timer", "enabled", "active", []string{"daemon-reload", "start hostdiary-collect.service", "enable --now hostdiary-collect.timer"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.fail, f.enabled, f.active = tc.fail, tc.enabled, tc.active
			if code := Install(f.env); code != 1 {
				t.Fatal(code)
			}
			calls := append([]string{}, tc.calls...)
			if tc.fail != "enable --now hostdiary-collect.timer" && len(calls) > 0 && calls[len(calls)-1] == "enable --now hostdiary-collect.timer" {
				calls = append(calls, "enable --now hostdiary-dirs.timer")
			}
			want := append(calls, "is-enabled hostdiary-collect.timer", "is-active hostdiary-collect.timer", "is-enabled hostdiary-dirs.timer", "is-active hostdiary-dirs.timer")
			if !reflect.DeepEqual(f.calls, want) {
				t.Fatal(f.calls)
			}
			for _, s := range []string{binaryPath + ": installed", servicePath + ": installed", timerPath + ": installed", collect.Version, "timer: enabled=" + tc.enabled + ", active="} {
				if !strings.Contains(f.out.String(), s) {
					t.Fatal(f.out.String())
				}
			}
			if tc.fail != "" && !strings.Contains(f.stderr.String(), "injected failure") {
				t.Fatal(f.stderr.String())
			}
			if strings.HasPrefix(tc.name, "start") {
				for _, s := range []string{"service diagnostics", "exit 1", "clock_backward", "systemctl enable --now"} {
					if !strings.Contains(f.stderr.String(), s) {
						t.Fatal(f.stderr.String())
					}
				}
			}
			if tc.active == "active" && tc.name != "query" && !strings.Contains(f.out.String(), "the timer is still active and runs the installed executable (version "+collect.Version+")") {
				t.Fatal(f.out.String())
			}
		})
	}
}

func TestInstallRepairsAndWarnings(t *testing.T) {
	f := newFixture(t)
	writeTestFile(t, f.path(binaryPath), "executable bytes", 0666)
	f.env.Self = f.path(binaryPath)
	writeTestFile(t, f.path(servicePath), "local changes", 0644)
	f.setOwner(t, servicePath, 123, 456)
	if err := os.Chmod(f.path("/usr/local/bin"), 0775|os.ModeSetgid); err != nil {
		t.Fatal(err)
	}
	if code := Install(f.env); code != 0 {
		t.Fatal(code, f.stderr.String())
	}
	for _, s := range []string{"fixed ownership/mode", "replaced (previous content differed; use systemctl edit for local changes)"} {
		if !strings.Contains(f.out.String(), s) {
			t.Fatal(f.out.String())
		}
	}
	if !strings.Contains(f.stderr.String(), "group '") || !strings.Contains(f.stderr.String(), "could replace the executable that runs as root") {
		t.Fatal(f.stderr.String())
	}
	f.out.Reset()
	if code := Install(f.env); code != 0 || !strings.Contains(f.out.String(), binaryPath+": same") {
		t.Fatal(code, f.out.String())
	}
	if !strings.Contains(f.out.String(), servicePath+": unchanged") {
		t.Fatal(f.out.String())
	}
}

func TestProductionRunRefusesTestRoot(t *testing.T) {
	// Even a valid executable must never run with a non-production root.
	_, _, code, err := ProductionRun(t.TempDir(), "/must-not-run", "daemon-reload")
	if code == 0 || err == nil || !strings.Contains(err.Error(), "root") {
		t.Fatal(code, err)
	}
}
