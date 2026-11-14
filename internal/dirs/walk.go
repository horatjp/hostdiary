package dirs

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

type Filesystem struct {
	Mount    string  `json:"mount"`
	Total    uint64  `json:"total"`
	Complete bool    `json:"complete"`
	Errors   int     `json:"errors"`
	Entries  []Entry `json:"entries"`
}

// One logger is shared across mounts so a large failing walk cannot flood stderr.
type walkErrors struct {
	stderr io.Writer
	errors int
}

func (l *walkErrors) record(p string, err error) {
	l.errors++
	if l.errors <= 100 {
		fmt.Fprintf(l.stderr, "dirs walk %q: %v\n", p, err)
	}
}
func (l *walkErrors) summary() {
	if l.errors > 0 {
		fmt.Fprintf(l.stderr, "dirs walk: errors=%d\n", l.errors)
	}
}

func walkWithLog(root, mount string, lstat func(string) (os.FileInfo, error), log *walkErrors) Filesystem {
	result := Filesystem{Mount: escapePath(mount), Complete: true, Entries: []Entry{}}
	recordError := func(p string, err error) {
		result.Errors++
		if !errors.Is(err, fs.ErrNotExist) && !errors.Is(err, syscall.ENOTDIR) {
			result.Complete = false
		}
		log.record(p, err)
	}
	fi, err := lstat(root)
	if err != nil {
		recordError(root, err)
		result.Complete = false
		return result
	}
	initial, err := statEntry(fi)
	if err != nil {
		recordError(root, err)
		return result
	}
	if !fi.IsDir() {
		recordError(root, fmt.Errorf("mount is not a directory"))
		return result
	}
	a := newAccumulator(mount)
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			recordError(p, walkErr)
			return nil
		}
		fi, err := lstat(p)
		if err != nil {
			recordError(p, err)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		e, err := statEntry(fi)
		if err != nil {
			recordError(p, err)
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if e.dev != initial.dev {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// If a directory became a symlink since ReadDir, never descend through it.
		replaced := d.IsDir() && !fi.IsDir()
		e.path, err = filepath.Rel(root, p)
		if err == nil {
			err = a.Add(e)
		}
		if err != nil {
			recordError(p, err)
		}
		if replaced {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		recordError(root, err)
	}
	usage := a.Finish(top)
	result.Total, result.Entries = usage.Total, usage.Entries
	return result
}
