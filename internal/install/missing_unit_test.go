package install

import (
	"os/exec"
	"strings"
	"testing"
)

const missingUnitDiagnostic = "Failed to get unit file state for hostdiary-collect.timer: No such file or directory\n"

// Reproduce systemd 252's replies without starting a real child process. Run
// carries the numeric exit code separately, so ExitError needs no ProcessState.
func missingTimerReplies(f *fixture) {
	run := f.env.Run
	f.env.Run = func(name string, args ...string) ([]byte, []byte, int, error) {
		switch args[0] {
		case "is-enabled":
			f.calls = append(f.calls, strings.Join(args, " "))
			return nil, []byte(missingUnitDiagnostic), 1, &exec.ExitError{}
		case "is-active":
			f.calls = append(f.calls, strings.Join(args, " "))
			return []byte("inactive\n"), nil, 3, &exec.ExitError{}
		}
		return run(name, args...)
	}
}

func TestUninstallMissingTimerRepliesAreNormal(t *testing.T) {
	for _, installed := range []bool{false, true} {
		name := "empty"
		if installed {
			name = "installed"
		}
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			if installed {
				if code := Install(f.env); code != 0 {
					t.Fatal(code)
				}
			}
			f.out.Reset()
			f.stderr.Reset()
			f.calls = nil
			missingTimerReplies(f)
			if code := Uninstall(f.env); code != 0 {
				t.Fatalf("code=%d, stderr=%s", code, f.stderr.String())
			}
			if !strings.Contains(f.out.String(), "timer: active=inactive") {
				t.Fatal(f.out.String())
			}
			if f.stderr.Len() != 0 {
				t.Fatal(f.stderr.String())
			}
			for _, call := range f.calls {
				if strings.HasPrefix(call, "is-enabled ") {
					t.Fatal(f.calls)
				}
			}
		})
	}
}

func TestInstallMissingTimerRepliesStillFailWithoutInactiveError(t *testing.T) {
	f := newFixture(t)
	missingTimerReplies(f)
	if code := Install(f.env); code != 1 {
		t.Fatal(code)
	}
	if !strings.Contains(f.out.String(), "timer: enabled=unknown, active=inactive") {
		t.Fatal(f.out.String())
	}
	if strings.Contains(f.stderr.String(), "systemctl is-active failed") {
		t.Fatal(f.stderr.String())
	}
}

func TestUninstallActiveTimerFailsEvenWithoutFiles(t *testing.T) {
	f := newFixture(t)
	if code := Uninstall(f.env); code != 1 {
		t.Fatal(code)
	}
	if !strings.Contains(f.out.String(), "active=active") || !strings.Contains(f.out.String(), "Stop it manually") {
		t.Fatal(f.out.String())
	}
	for _, call := range f.calls {
		if strings.HasPrefix(call, "disable ") {
			t.Fatal("disable called without timer unit")
		}
	}
}
