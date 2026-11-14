package install

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

const replaced = "replaced (previous content differed; use systemctl edit for local changes)"

// placeFile preserves matching regular files and replaces every other regular
// file atomically, detaching old links/fds. It never follows destination symlinks.
func placeFile(env Env, path string, content []byte, mode os.FileMode) (string, error) {
	fi, err := os.Lstat(path)
	exists := err == nil
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	status := "installed"
	if exists {
		if !fi.Mode().IsRegular() {
			if fi.Mode()&os.ModeSymlink != 0 && (filepath.Base(path) == serviceName || filepath.Base(path) == timerName) {
				if target, err := os.Readlink(path); err == nil && target == "/dev/null" {
					return "", fmt.Errorf("masked; run systemctl unmask %s first", filepath.Base(path))
				}
			}
			return "", fmt.Errorf("destination is not a regular file; left unchanged")
		}
		previous, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		status = replaced
		if bytes.Equal(previous, content) {
			if metadataMatches(env, fi, mode) {
				return "unchanged", nil
			}
			status = "fixed ownership/mode"
		}
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".hostdiary-*")
	if err != nil {
		return "", err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err := f.Write(content); err != nil {
		return "", err
	}
	if err := env.Chown(f, 0, 0); err != nil {
		return "", err
	}
	if err := f.Chmod(mode); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := renameAndSync(f.Name(), path, syncDirectory); err != nil {
		return "", err
	}
	return status, nil
}

// Keep rename and directory durability in order; a sync failure is operational
// failure even though the new file is already visible and cannot be rolled back.
func renameAndSync(tmp, path string, syncDir func(string) error) error {
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	return syncDir(filepath.Dir(path))
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// metadataMatches requires exact modes, including setuid, setgid and sticky.
func metadataMatches(env Env, fi os.FileInfo, mode os.FileMode) bool {
	uid, gid := env.Owner(fi)
	return uid == 0 && gid == 0 && fi.Mode() == mode
}

func createConfigDirectory(env Env) error {
	path := env.path("/etc/hostdiary")
	if _, err := os.Stat(path); err == nil {
		return requireDirectory(path)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Mkdir(path, 0755); err != nil {
		return err
	}
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err := env.Chown(dir, 0, 0); err != nil {
		return fmt.Errorf("config directory ownership: %w", err)
	}
	return dir.Chmod(0755)
}
