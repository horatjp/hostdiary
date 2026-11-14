package dirs

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestNonDirectoryHardLinksCountOnce(t *testing.T) {
	a := newAccumulator("/")
	for _, e := range []entry{{path: "a/link", bytes: 512, ino: 7, nlink: 2}, {path: "b/link", bytes: 512, ino: 7, nlink: 2}} {
		if err := a.Add(e); err != nil {
			t.Fatal(err)
		}
	}
	if got := a.Finish(20); got.Total != 512 {
		t.Fatal(got)
	}
}

func TestPruneJSONBeforeFailedJSDeletion(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			dir := t.TempDir()
			for i := 0; i < 32; i++ {
				day := clock().AddDate(0, 0, i).Format("2006-01-02")
				for _, ext := range []string{".json", ".js"} {
					writeFixture(t, dir, day+ext, []byte("data"))
				}
			}
			calls := []string{}
			err := pruneFilesWithRemove(dir, disabled, "", func(p string) error {
				calls = append(calls, filepath.Base(p))
				if strings.HasSuffix(p, ".js") {
					return fs.ErrPermission
				}
				return os.Remove(p)
			})
			if !errors.Is(err, fs.ErrPermission) {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(dir, "2026-11-01.json")); !os.IsNotExist(err) {
				t.Fatal("JSON still advertised", calls, err)
			}
			if _, err := os.Stat(filepath.Join(dir, "2026-11-01.js")); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNoSelectedFilesystemPreservesExistingDay(t *testing.T) {
	for _, mode := range []string{"mountinfo", "filter"} {
		t.Run(mode, func(t *testing.T) {
			root := setup(t)
			if mode == "mountinfo" {
				writeFixture(t, root, "proc/self/mountinfo", []byte(""))
			} else {
				writeFixture(t, root, "etc/hostdiary/config.json", []byte(`{"disk":{"exclude":["/fixture"]}}`))
			}
			for _, ext := range []string{".json", ".js"} {
				writeFixture(t, root, "var/lib/hostdiary/public/data/dirs/2026-11-01"+ext, []byte("sentinel"))
			}
			var log bytes.Buffer
			if code := Run(root, &log, clock); code != 0 {
				t.Fatal(code, log.String())
			}
			if strings.Count(log.String(), "no filesystem selected\n") != 1 {
				t.Fatal(log.String())
			}
			for _, ext := range []string{".json", ".js"} {
				b, err := os.ReadFile(filepath.Join(root, "var/lib/hostdiary/public/data/dirs/2026-11-01"+ext))
				if err != nil || string(b) != "sentinel" {
					t.Fatal(string(b), err)
				}
			}
		})
	}
}

func TestENOTDIRAndBoundedWalkErrors(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 105; i++ {
		writeFixture(t, root, fmt.Sprintf("gone%03d", i), nil)
	}
	var log bytes.Buffer
	got := walk(root, "/fixture", func(p string) (os.FileInfo, error) {
		if p != root {
			return nil, &os.PathError{Op: "lstat", Path: p, Err: syscall.ENOTDIR}
		}
		return os.Lstat(p)
	}, &log)
	if !got.Complete || got.Errors != 105 {
		t.Fatal(got)
	}
	if strings.Count(log.String(), "dirs walk \"") != 100 || !strings.HasSuffix(log.String(), "dirs walk: errors=105\n") {
		t.Fatal(log.String())
	}
}

func TestRunPartialPublishesAndCapsErrorsAcrossFilesystems(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("chmod does not deny root")
	}
	root := setup(t)
	writeFixture(t, root, "proc/self/mountinfo", []byte("1 0 8:1 / /fixture rw - ext4 /dev/fixture rw\n2 0 8:2 / /second rw - ext4 /dev/second rw\n"))
	for _, mount := range []string{"fixture", "second"} {
		for i := 0; i < 60; i++ {
			p := filepath.Join(root, mount, fmt.Sprintf("blocked%03d", i))
			if err := os.MkdirAll(p, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(p, 0000); err != nil {
				t.Fatal(err)
			}
			defer os.Chmod(p, 0755)
		}
	}
	var log bytes.Buffer
	if code := Run(root, &log, clock); code != 1 {
		t.Fatal(code, log.String())
	}
	if strings.Count(log.String(), "dirs walk \"") != 100 || !strings.Contains(log.String(), "dirs walk: errors=120\n") {
		t.Fatal(log.String())
	}
	b, err := os.ReadFile(filepath.Join(root, "var/lib/hostdiary/public/data/dirs/2026-11-01.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc Document
	if err = json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if len(doc.Filesystems) != 2 {
		t.Fatal(doc)
	}
	for _, f := range doc.Filesystems {
		if f.Complete || f.Errors != 60 {
			t.Fatal(f)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "var/lib/hostdiary/public/data/dirs/2026-11-01.js")); err != nil {
		t.Fatal(err)
	}
}
