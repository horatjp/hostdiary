package collect

import (
	"bytes"
	"encoding/json"
	"hostdiary/internal/store"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func copyFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	e := filepath.WalkDir("../../testdata", func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		rel, e := filepath.Rel("../../testdata", p)
		if e != nil {
			return e
		}
		dest := filepath.Join(root, rel)
		if d.IsDir() {
			return os.MkdirAll(dest, 0755)
		}
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		return os.WriteFile(dest, b, 0644)
	})
	if e != nil {
		t.Fatal(e)
	}
	return root
}
func write(t *testing.T, root, p, s string) {
	t.Helper()
	if e := os.WriteFile(filepath.Join(root, p), []byte(s), 0644); e != nil {
		t.Fatal(e)
	}
}
func runAt(root string, at int64) (int, string) {
	var log bytes.Buffer
	code := Run(root, &log, func() time.Time { return time.Unix(at, 0) })
	return code, log.String()
}
func index(t *testing.T, root string) store.Index {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(root, "var/lib/hostdiary/public/index.json"))
	if e != nil {
		t.Fatal(e)
	}
	var idx store.Index
	if e = json.Unmarshal(b, &idx); e != nil {
		t.Fatal(e)
	}
	return idx
}
func TestTwoCollections(t *testing.T) {
	root := copyFixture(t)
	at := int64(1793404800)
	if code, log := runAt(root, at); code != 0 {
		t.Fatal(code, log)
	}
	write(t, root, "proc/uptime", "1060 1900\n")
	write(t, root, "proc/net/dev", "eth0: 1600 10 0 0 0 0 0 0 3200 20 0 0 0 0 0 0\n")
	write(t, root, "proc/stat", "cpu 110 30 40 410 20 15 15 10\ncpu0 1\ncpu1 1\n")
	if code, log := runAt(root, at+120); code != 0 {
		t.Fatal(code, log)
	}
	idx := index(t, root)
	if idx.Host != "fixture-host" || idx.GeneratedAt != at+120 || len(idx.Boots) != 1 {
		t.Fatalf("%+v", idx)
	}
	id := store.GroupID("net", "eth0")
	b, e := os.ReadFile(filepath.Join(root, "var/lib/hostdiary/public/data/1m/2026-10-31", id+".jsonl"))
	if e != nil {
		t.Fatal(e)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 3 {
		t.Fatal(string(b))
	}
	var first, last []any
	json.Unmarshal([]byte(lines[1]), &first)
	json.Unmarshal([]byte(lines[2]), &last)
	if first[1] != nil || last[1] != float64(10) || last[2] != float64(20) {
		t.Fatal(first, last)
	}
	state, _ := os.ReadFile(filepath.Join(root, "var/lib/hostdiary/state.json"))
	if code, log := runAt(root, at+120); code != 1 || !strings.Contains(log, "clock_backward") {
		t.Fatal(code, log)
	}
	after, _ := os.ReadFile(filepath.Join(root, "var/lib/hostdiary/state.json"))
	if !bytes.Equal(state, after) {
		t.Fatal("state advanced")
	}
	after, _ = os.ReadFile(filepath.Join(root, "var/lib/hostdiary/public/data/1m/2026-10-31", id+".jsonl"))
	if !bytes.Equal(b, after) {
		t.Fatal("clock backward appended")
	}
}
func TestCorruptStateAndReboot(t *testing.T) {
	root := copyFixture(t)
	at := int64(1793404800)
	if c, l := runAt(root, at); c != 0 {
		t.Fatal(c, l)
	}
	write(t, root, "var/lib/hostdiary/state.json", "broken")
	if c, l := runAt(root, at+60); c != 0 || !strings.Contains(l, "state ignored") {
		t.Fatal(c, l)
	}
	write(t, root, "proc/sys/kernel/random/boot_id", "00000000-0000-4000-8000-000000000002\n")
	write(t, root, "proc/uptime", "10 10\n")
	if c, l := runAt(root, at+120); c != 0 {
		t.Fatal(c, l)
	}
	if got := index(t, root).Boots; len(got) != 2 || got[1] != at+110 {
		t.Fatal(got)
	}
}
func TestInvalidConfigWritesNothing(t *testing.T) {
	root := copyFixture(t)
	write(t, root, "etc/hostdiary/config.json", `{"bad":true}`)
	if c, l := runAt(root, 1793404800); c != 2 {
		t.Fatal(c, l)
	}
	if _, e := os.Stat(filepath.Join(root, "var")); !os.IsNotExist(e) {
		t.Fatal("invalid config wrote state")
	}
}
func TestLocked(t *testing.T) {
	root := copyFixture(t)
	base := filepath.Join(root, "var/lib/hostdiary")
	os.MkdirAll(base, 0755)
	f, e := os.Create(filepath.Join(base, "collect.lock"))
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	if e = lock(f); e != nil {
		t.Fatal(e)
	}
	if c, l := runAt(root, 1793404800); c != 75 {
		t.Fatal(c, l)
	}
	if _, e = os.Stat(filepath.Join(base, "state.json")); !os.IsNotExist(e) {
		t.Fatal("locked run wrote state")
	}
}
func TestPublicPermissionsAndPrivateState(t *testing.T) {
	root := copyFixture(t)
	old := syscall.Umask(0077)
	defer syscall.Umask(old)
	if c, l := runAt(root, 1793404800); c != 0 {
		t.Fatal(c, l)
	}
	base := filepath.Join(root, "var/lib/hostdiary")
	e := filepath.WalkDir(filepath.Join(base, "public"), func(p string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		st, e := d.Info()
		if e != nil {
			return e
		}
		want := fs.FileMode(0644)
		if d.IsDir() {
			want = 0755
		}
		if st.Mode().Perm() != want {
			t.Errorf("%s mode %o", filepath.Base(p), st.Mode().Perm())
		}
		return nil
	})
	if e != nil {
		t.Fatal(e)
	}
	st, _ := os.Stat(filepath.Join(base, "state.json"))
	if st.Mode().Perm() != 0600 {
		t.Fatal(st.Mode())
	}
}
func TestUnavailableAndUnexpectedFailure(t *testing.T) {
	for _, bad := range []bool{false, true} {
		t.Run(map[bool]string{false: "unsupported", true: "error"}[bad], func(t *testing.T) {
			root := copyFixture(t)
			if bad {
				write(t, root, "proc/loadavg", "invalid")
			} else {
				os.Remove(filepath.Join(root, "proc/loadavg"))
			}
			c, l := runAt(root, 1793404800)
			if bad && c != 1 || !bad && c != 0 {
				t.Fatal(c, l)
			}
			idx := index(t, root)
			found := false
			for _, u := range idx.Unavailable {
				if u.Group == "load" {
					found = true
					if bad && u.Reason != "error" || !bad && u.Reason != "unsupported" {
						t.Fatal(u)
					}
				}
			}
			if !found {
				t.Fatal("missing unavailable")
			}
		})
	}
}
