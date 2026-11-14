package dirs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func clock() time.Time { return time.Date(2026, 11, 1, 23, 59, 0, 0, time.UTC) }
func setup(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFixture(t, root, "proc/self/mountinfo", []byte("1 0 8:1 / /fixture rw - ext4 /dev/fixture rw\n2 0 8:1 / /longer-fixture rw - ext4 /dev/fixture rw\n3 0 0:1 / /virtual rw - tmpfs tmpfs rw\n"))
	writeFixture(t, root, "fixture/a/file", []byte("test"))
	return root
}
func TestRunPublishesAndReplacesWithoutCollectChanges(t *testing.T) {
	root := setup(t)
	base := "var/lib/hostdiary/"
	writeFixture(t, root, base+"collect.lock", []byte("sentinel"))
	writeFixture(t, root, base+"public/index.json", []byte("sentinel"))
	var log bytes.Buffer
	calls := 0
	now := func() time.Time { calls++; return clock().Add(time.Duration(calls-1) * time.Minute) }
	if code := Run(root, &log, now); code != 0 {
		t.Fatal(code, log.String())
	}
	p := filepath.Join(root, base+"public/data/dirs/2026-11-01.json")
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var doc Document
	if err = json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Schema != 1 || doc.Kind != "dirs" || doc.Depth != 2 || doc.Top != 20 || len(doc.Filesystems) != 1 || doc.StartedAt != clock().Unix() || doc.FinishedAt != clock().Add(time.Minute).Unix() {
		t.Fatal(doc)
	}
	js, err := os.ReadFile(strings.TrimSuffix(p, ".json") + ".js")
	if err != nil {
		t.Fatal(err)
	}
	if string(js) != "HD.dirs(\"2026-11-01\", "+strings.TrimSpace(string(b))+");\n" {
		t.Fatal(string(js))
	}
	for _, name := range []string{"collect.lock", "public/index.json"} {
		b, _ := os.ReadFile(filepath.Join(root, base+name))
		if string(b) != "sentinel" {
			t.Fatal(name)
		}
	}
	for _, name := range []string{p, strings.TrimSuffix(p, ".json") + ".js"} {
		fi, _ := os.Stat(name)
		if fi.Mode().Perm() != 0644 {
			t.Fatal(fi.Mode())
		}
	}
	writeFixture(t, root, "fixture/a/new", bytes.Repeat([]byte("x"), 8192))
	if code := Run(root, &log, clock); code != 0 {
		t.Fatal(code, log.String())
	}
	b2, _ := os.ReadFile(p)
	if bytes.Equal(b, b2) {
		t.Fatal("not replaced")
	}
}

func TestRunDisabledDeletesOnlyDatedFilesWithoutWalking(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "etc/hostdiary/config.json", []byte(`{"dirs":{"enabled":false}}`))
	dir := "var/lib/hostdiary/public/data/dirs/"
	for _, name := range []string{"2026-11-01.json", "2026-10-31.js", "notes.json", "2026-02-30.json", "2026-11-01.txt"} {
		writeFixture(t, root, dir+name, []byte("keep?"))
	}
	var log bytes.Buffer
	if code := Run(root, &log, clock); code != 0 {
		t.Fatal(code, log.String())
	}
	for _, name := range []string{"2026-11-01.json", "2026-10-31.js"} {
		if _, err := os.Lstat(filepath.Join(root, dir+name)); !os.IsNotExist(err) {
			t.Fatal(name, err)
		}
	}
	for _, name := range []string{"notes.json", "2026-02-30.json", "2026-11-01.txt"} {
		if _, err := os.Stat(filepath.Join(root, dir+name)); err != nil {
			t.Fatal(name, err)
		}
	}
}

func TestRetentionKeepsNewest31ByName(t *testing.T) {
	root := setup(t)
	dir := "var/lib/hostdiary/public/data/dirs/"
	for i := 0; i < 34; i++ {
		day := clock().AddDate(0, 0, i).Format("2006-01-02")
		for _, ext := range []string{".json", ".js"} {
			writeFixture(t, root, dir+day+ext, []byte("old"))
		}
	}
	writeFixture(t, root, dir+"notes.json", []byte("keep"))
	var log bytes.Buffer
	if code := Run(root, &log, clock); code != 0 {
		t.Fatal(code, log.String())
	}
	items, _ := os.ReadDir(filepath.Join(root, dir))
	if len(items) != 65 {
		t.Fatal(len(items))
	}
	if _, err := os.Stat(filepath.Join(root, dir+"2026-11-01.json")); err != nil {
		t.Fatal(err)
	}
}

func TestRunConfigLockAndReadFailures(t *testing.T) {
	t.Run("config", func(t *testing.T) {
		root := t.TempDir()
		writeFixture(t, root, "etc/hostdiary/config.json", []byte(`{"dirs":{"bad":true}}`))
		if code := Run(root, &bytes.Buffer{}, clock); code != 2 {
			t.Fatal(code)
		}
		if _, err := os.Stat(filepath.Join(root, "var")); !os.IsNotExist(err) {
			t.Fatal(err)
		}
	})
	t.Run("lock", func(t *testing.T) {
		root := setup(t)
		p := writeFixture(t, root, "var/lib/hostdiary/dirs.lock", nil)
		f, err := os.OpenFile(p, os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			t.Fatal(err)
		}
		if code := Run(root, &bytes.Buffer{}, clock); code != 75 {
			t.Fatal(code)
		}
	})
	for _, text := range []string{"bad mountinfo", ""} {
		t.Run(fmt.Sprintf("mountinfo:%q", text), func(t *testing.T) {
			root := setup(t)
			writeFixture(t, root, "proc/self/mountinfo", []byte(text))
			code := Run(root, &bytes.Buffer{}, clock)
			want := 0
			if text != "" {
				want = 1
			}
			if code != want {
				t.Fatal(code)
			}
		})
	}
	t.Run("missing mount", func(t *testing.T) {
		root := t.TempDir()
		writeFixture(t, root, "proc/self/mountinfo", []byte("1 0 8:1 / /missing rw - ext4 /dev/fixture rw\n"))
		if code := Run(root, &bytes.Buffer{}, clock); code != 1 {
			t.Fatal(code)
		}
	})
	t.Run("filter", func(t *testing.T) {
		root := setup(t)
		writeFixture(t, root, "etc/hostdiary/config.json", []byte(`{"disk":{"exclude":["/fixture"]}}`))
		if code := Run(root, &bytes.Buffer{}, clock); code != 0 {
			t.Fatal(code)
		}
		if _, err := os.Stat(filepath.Join(root, "var/lib/hostdiary/public/data/dirs/2026-11-01.json")); !os.IsNotExist(err) {
			t.Fatal(err)
		}
	})
}
