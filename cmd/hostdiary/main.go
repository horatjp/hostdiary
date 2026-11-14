// hostdiary dispatches commands; collection behavior lives in internal/collect.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"hostdiary/internal/collect"
	"hostdiary/internal/dirs"
	"hostdiary/internal/install"
)

const usage = "usage: hostdiary collect | dirs | install | uninstall | --version"

func run(args []string, stderr io.Writer, stdout io.Writer) int {
	return runWithInstallEnv(args, stderr, stdout, installEnv)
}

func runWithInstallEnv(args []string, stderr, stdout io.Writer, makeEnv func(io.Writer, io.Writer) install.Env) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Fprintln(stdout, collect.Version)
		return 0
	}
	if len(args) > 0 && (args[0] == "install" || args[0] == "uninstall") {
		if len(args) != 1 {
			fmt.Fprintln(stderr, usage)
			return 2
		}
		env := makeEnv(stderr, stdout)
		if args[0] == "uninstall" {
			return install.Uninstall(env)
		}
		return install.Install(env)
	}
	if len(args) == 0 || args[0] != "collect" && args[0] != "dirs" {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	fs := flag.NewFlagSet(args[0], flag.ContinueOnError)
	fs.SetOutput(stderr)
	root := fs.String("root", "/", "development filesystem root")
	fs.Usage = func() { fmt.Fprintln(stderr, usage) }
	if e := fs.Parse(args[1:]); e != nil {
		return 2
	}
	if fs.NArg() != 0 {
		return 2
	}
	if args[0] == "dirs" {
		return dirs.Run(*root, stderr, time.Now)
	}
	return collect.Run(*root, stderr, time.Now)
}

func installEnv(stderr, stdout io.Writer) install.Env {
	self, err := os.Executable()
	if err == nil {
		self, err = filepath.EvalSymlinks(self)
	}
	if err != nil {
		self = ""
	}
	const root = "/"
	return install.Env{
		Root: root, UID: os.Geteuid(), Self: self, Systemctl: findSystemctl(root),
		Run: func(name string, args ...string) ([]byte, []byte, int, error) {
			return install.ProductionRun(root, name, args...)
		},
		Chown: func(f *os.File, uid, gid int) error { return f.Chown(uid, gid) },
		Owner: func(fi os.FileInfo) (int, int) {
			stat := fi.Sys().(*syscall.Stat_t)
			return int(stat.Uid), int(stat.Gid)
		},
		Stdout: stdout, Stderr: stderr,
	}
}

// Fixed paths keep root execution independent of the invoking user's PATH.
func findSystemctl(root string) string {
	for _, path := range []string{"/usr/bin/systemctl", "/bin/systemctl"} {
		full := filepath.Join(root, path)
		fi, err := os.Stat(full)
		if err == nil && fi.Mode().IsRegular() && fi.Mode().Perm()&0111 != 0 {
			return full
		}
	}
	return ""
}
func main() { os.Exit(run(os.Args[1:], os.Stderr, os.Stdout)) }
