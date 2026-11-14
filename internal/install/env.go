// Package install places the executable and systemd units without changing user data.
package install

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const (
	binaryPath      = "/usr/local/bin/hostdiary"
	servicePath     = "/etc/systemd/system/hostdiary-collect.service"
	timerPath       = "/etc/systemd/system/hostdiary-collect.timer"
	serviceName     = "hostdiary-collect.service"
	timerName       = "hostdiary-collect.timer"
	dirsServiceName = "hostdiary-dirs.service"
	dirsTimerName   = "hostdiary-dirs.timer"
	dirsServicePath = "/etc/systemd/system/hostdiary-dirs.service"
	dirsTimerPath   = "/etc/systemd/system/hostdiary-dirs.timer"
)

// Env keeps filesystem and command side effects explicit. Tests supply every hook.
// Root is not a CLI option for install or uninstall.
type Env struct {
	Root           string
	UID            int
	Self           string
	Systemctl      string
	Run            func(name string, args ...string) (stdout, stderr []byte, exitCode int, err error)
	Chown          func(f *os.File, uid, gid int) error
	Owner          func(fi os.FileInfo) (uid, gid int)
	Stdout, Stderr io.Writer
}

// ProductionRun must be bound to the Env.Root by the caller. There is deliberately
// no automatic Run fallback: a test filesystem must never control the host manager.
func ProductionRun(root, name string, args ...string) ([]byte, []byte, int, error) {
	if root != "/" {
		return nil, nil, 2, fmt.Errorf("refusing production command with root %q", root)
	}
	return runWithTimeout(commandTimeout, func(ctx context.Context) ([]byte, []byte, int, error) {
		var stdout, stderr bytes.Buffer
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		// Bound pipe cleanup too, if a descendant keeps an output descriptor open.
		cmd.WaitDelay = time.Second
		err := cmd.Run()
		code := 0
		if err != nil {
			code = 1
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			}
		}
		return stdout.Bytes(), stderr.Bytes(), code, err
	})
}

const commandTimeout = 90 * time.Second

// The runner must honor ctx (production uses CommandContext). Deadline expiry
// always fails, even when a runner reports a successful exit concurrently.
func runWithTimeout(timeout time.Duration, run func(context.Context) ([]byte, []byte, int, error)) ([]byte, []byte, int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stdout, stderr, code, err := run(ctx)
	if ctx.Err() != nil {
		return stdout, stderr, 1, ctx.Err()
	}
	return stdout, stderr, code, err
}

func (e Env) path(path string) string { return filepath.Join(e.Root, path) }

func (e Env) writers() Env {
	if e.Stdout == nil {
		e.Stdout = io.Discard
	}
	if e.Stderr == nil {
		e.Stderr = io.Discard
	}
	return e
}

func (e Env) check(install bool) error {
	if !filepath.IsAbs(e.Root) {
		return fmt.Errorf("filesystem root must be absolute")
	}
	if e.UID != 0 {
		return fmt.Errorf("run as root")
	}
	if err := requireDirectory(e.path("/run/systemd/system")); err != nil {
		return fmt.Errorf("systemd is not running: %w", err)
	}
	if e.Systemctl == "" {
		return fmt.Errorf("systemctl was not found")
	}
	if e.Run == nil {
		return fmt.Errorf("command runner is required")
	}
	if install {
		if e.Chown == nil || e.Owner == nil {
			return fmt.Errorf("ownership hooks are required")
		}
		for _, path := range []string{"/usr/local/bin", "/etc/systemd/system"} {
			if err := requireDirectory(e.path(path)); err != nil {
				return err
			}
		}
		fi, err := os.Stat(e.Self)
		if err != nil {
			return fmt.Errorf("cannot read executable: %w", err)
		}
		if !fi.Mode().IsRegular() {
			return fmt.Errorf("executable is not a regular file")
		}
	}
	return nil
}

func requireDirectory(path string) error {
	fi, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s is not a directory", path)
	}
	return nil
}

func (e Env) command(args ...string) bool {
	output, diagnostics, code, err := e.Run(e.Systemctl, args...)
	if code == 0 && err == nil {
		if len(diagnostics) > 0 {
			fmt.Fprintf(e.Stderr, "%s\n", diagnostics)
		}
		return true
	}
	fmt.Fprintf(e.Stderr, "systemctl %v failed (exit %d): %v\n%s\n%s\n", args, code, err, output, diagnostics)
	return false
}
