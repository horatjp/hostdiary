package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hostdiary/internal/install"
)

func TestDispatch(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
	}{{nil, 2}, {[]string{"unknown"}, 2}, {[]string{"--version"}, 0}, {[]string{"collect", "extra"}, 2}, {[]string{"collect", "--bad"}, 2}} {
		var out, err bytes.Buffer
		if got := run(tc.args, &err, &out); got != tc.code {
			t.Fatal(tc.args, got)
		}
		if tc.code == 0 && !strings.Contains(out.String(), "0.3.0") {
			t.Fatal(out.String())
		}
	}
}

func TestInstallUninstallDispatchWithFakeSystem(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{"run/systemd/system", "usr/local/bin", "etc/systemd/system"} {
		if err := os.MkdirAll(filepath.Join(root, path), 0755); err != nil {
			t.Fatal(err)
		}
	}
	self := filepath.Join(root, "source")
	if err := os.WriteFile(self, []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	var calls []string
	factory := func(stderr, stdout io.Writer) install.Env {
		return install.Env{
			Root: root, UID: 0, Self: self, Systemctl: "fake",
			Chown: func(*os.File, int, int) error { return nil }, Owner: func(os.FileInfo) (int, int) { return 0, 0 },
			Run: func(_ string, args ...string) ([]byte, []byte, int, error) {
				calls = append(calls, strings.Join(args, " "))
				if args[0] == "is-enabled" {
					if _, err := os.Stat(filepath.Join(root, "etc/systemd/system/hostdiary-collect.timer")); os.IsNotExist(err) {
						return nil, []byte("Failed to get unit file state: No such file or directory\n"), 1, nil
					}
					return []byte("enabled\n"), nil, 0, nil
				}
				if args[0] == "is-active" {
					if _, err := os.Stat(filepath.Join(root, "etc/systemd/system/hostdiary-collect.timer")); os.IsNotExist(err) {
						return []byte("inactive\n"), nil, 3, nil
					}
					return []byte("active\n"), nil, 0, nil
				}
				return nil, nil, 0, nil
			}, Stdout: stdout, Stderr: stderr,
		}
	}
	for _, command := range []string{"install", "uninstall"} {
		var out, stderr bytes.Buffer
		if code := runWithInstallEnv([]string{command}, &stderr, &out, factory); code != 0 {
			t.Fatal(command, code, stderr.String())
		}
	}
	if len(calls) != 14 || calls[8] != "disable --now hostdiary-collect.timer" {
		t.Fatal(calls)
	}
	if _, err := os.Stat(filepath.Join(root, "usr/local/bin/hostdiary")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestInstallArgumentsRejectedBeforeDispatch(t *testing.T) {
	for _, args := range [][]string{
		{"install", "--root", "/tmp/x"}, {"uninstall", "--root", "/tmp/x"},
		{"install", "extra"}, {"uninstall", "extra"}, {"install", "--help"},
	} {
		var out, stderr bytes.Buffer
		if code := run(args, &stderr, &out); code != 2 {
			t.Fatal(args, code)
		}
		if !strings.Contains(stderr.String(), "usage: hostdiary collect | dirs | install | uninstall | --version") {
			t.Fatal(stderr.String())
		}
	}
}

func TestSystemctlFixedPathsIgnorePATH(t *testing.T) {
	root := t.TempDir()
	decoy := filepath.Join(root, "decoy")
	if err := os.MkdirAll(decoy, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decoy, "systemctl"), []byte("must never run"), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", decoy)
	if got := findSystemctl(root); got != "" {
		t.Fatal("used PATH", got)
	}
	for _, path := range []string{"bin/systemctl", "usr/bin/systemctl"} {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("not executed"), 0755); err != nil {
			t.Fatal(err)
		}
		if got := findSystemctl(root); got != full {
			t.Fatal(got, full)
		}
	}
	if err := os.Chmod(filepath.Join(root, "usr/bin/systemctl"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := findSystemctl(root); got != filepath.Join(root, "bin/systemctl") {
		t.Fatal(got)
	}
}
