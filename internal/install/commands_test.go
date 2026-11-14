package install

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestCommandDeadlineAndSeparateOutput(t *testing.T) {
	if commandTimeout != 90*time.Second {
		t.Fatal(commandTimeout)
	}
	out, diagnostics, code, err := runWithTimeout(commandTimeout, func(ctx context.Context) ([]byte, []byte, int, error) {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > commandTimeout || time.Until(deadline) < 89*time.Second {
			t.Fatal(deadline, ok)
		}
		return []byte("stdout"), []byte("stderr"), 0, nil
	})
	if string(out) != "stdout" || string(diagnostics) != "stderr" || code != 0 || err != nil {
		t.Fatal(string(out), string(diagnostics), code, err)
	}
}

func TestCommandDeadlineIsFailureEvenIfRunnerReturnsZero(t *testing.T) {
	_, _, code, err := runWithTimeout(time.Millisecond, func(ctx context.Context) ([]byte, []byte, int, error) {
		<-ctx.Done()
		return nil, nil, 0, nil
	})
	if code == 0 || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(code, err)
	}
}

func TestTimerStateUsesLastNonemptyStdoutLine(t *testing.T) {
	for _, state := range []string{"active", "inactive", "unknown"} {
		t.Run(state, func(t *testing.T) {
			f := newFixture(t)
			f.env.Run = func(string, ...string) ([]byte, []byte, int, error) {
				out := []byte("previous stdout line\n  " + state + " \n\n")
				if state == "unknown" {
					out = []byte(" \n\n")
				}
				code := 0
				if state == "inactive" {
					code = 3
				}
				return out, []byte("warning on stderr\nactive\n"), code, nil
			}
			got, ok, _ := timerState(f.env, "is-active", "active", timerName)
			if got != state || ok != (state == "active") {
				t.Fatal(got, ok)
			}
			if !strings.Contains(f.stderr.String(), "warning on stderr") {
				t.Fatal("discarded diagnostic")
			}
		})
	}
}

func TestUninstallQueryTimeoutFails(t *testing.T) {
	f := newFixture(t)
	run := f.env.Run
	f.env.Run = func(name string, args ...string) ([]byte, []byte, int, error) {
		if args[0] == "is-active" {
			return nil, nil, 1, context.DeadlineExceeded
		}
		return run(name, args...)
	}
	if code := Uninstall(f.env); code != 1 {
		t.Fatal(code)
	}
	if !strings.Contains(f.out.String(), "active=unknown") {
		t.Fatal(f.out.String())
	}
}

func TestTrialAdviceRequiresConfirmedExecMainStatusOne(t *testing.T) {
	for _, tc := range []struct {
		name, out string
		code      int
		want      bool
	}{
		{"one", "ExecMainStatus=1\n", 0, true}, {"timeout", "ExecMainStatus=0\n", 0, false},
		{"config", "ExecMainStatus=2\n", 0, false}, {"unknown", "", 1, false},
		{"failed-query", "ExecMainStatus=1\n", 1, false}, {"malformed", "ExecMainStatus=11\n", 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.fail = "start hostdiary-collect.service"
			run := f.env.Run
			f.env.Run = func(name string, args ...string) ([]byte, []byte, int, error) {
				if args[0] == "show" {
					f.calls = append(f.calls, strings.Join(args, " "))
					return []byte(tc.out), []byte("ExecMainStatus=1\n"), tc.code, nil
				}
				return run(name, args...)
			}
			if code := Install(f.env); code != 1 {
				t.Fatal(code)
			}
			if got := strings.Contains(f.stderr.String(), "collect exit 1 can include"); got != tc.want {
				t.Fatal(f.stderr.String())
			}
			if got := strings.Contains(f.stderr.String(), "If acceptable, run:"); got != tc.want {
				t.Fatal(f.stderr.String())
			}
			for _, call := range f.calls {
				if strings.HasPrefix(call, "enable ") {
					t.Fatal(f.calls)
				}
			}
		})
	}
}

func TestUninstallShowsRemainingActiveTimer(t *testing.T) {
	f := newFixture(t)
	if code := Install(f.env); code != 0 {
		t.Fatal(code)
	}
	f.out.Reset()
	f.fail = "disable --now hostdiary-collect.timer"
	if code := Uninstall(f.env); code != 1 {
		t.Fatal(code)
	}
	for _, s := range []string{"timer: active=active", "the timer is still active", "systemctl disable --now hostdiary-collect.timer"} {
		if !strings.Contains(f.out.String(), s) {
			t.Fatal(f.out.String())
		}
	}
}
