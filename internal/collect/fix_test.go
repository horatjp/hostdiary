package collect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// One broken mirror must leave that group's last_seen unchanged and allow others,
// the index and the next rate baseline to advance.
func TestAppendFailureContinuesCollection(t *testing.T) {
	root := copyFixture(t)
	at := int64(1793404800)
	if c, l := runAt(root, at); c != 0 {
		t.Fatal(c, l)
	}
	day := filepath.Join(root, "var/lib/hostdiary/public/data/1m/2026-10-31")
	if e := os.Remove(filepath.Join(day, "cpu.js")); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(filepath.Join(day, "cpu.js"), 0755); e != nil {
		t.Fatal(e)
	}
	write(t, root, "proc/uptime", "1060 1900\n")
	if c, l := runAt(root, at+60); c != 1 || !strings.Contains(l, "cpu") {
		t.Fatal(c, l)
	}
	idx := index(t, root)
	if idx.GeneratedAt != at+60 {
		t.Fatal(idx)
	}
	seen := map[string]int64{}
	for _, g := range idx.Groups {
		seen[g.ID] = g.LastSeen
	}
	if seen["cpu"] != at || seen["mem"] != at+60 || seen["tcp"] != at+60 {
		t.Fatal(seen)
	}
	var state State
	b, e := os.ReadFile(filepath.Join(root, "var/lib/hostdiary/state.json"))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &state); e != nil || state.LastCollected != at+60 || state.Observation.Uptime != 1060 {
		t.Fatal(state, e)
	}
}

func TestTCPFamilyAvailabilityRows(t *testing.T) {
	for _, tc := range []struct {
		name    string
		missing []string
		column  int
	}{
		{"v4 only", []string{"proc/net/tcp6"}, 1},
		{"v6 only", []string{"proc/net/tcp"}, 6},
		{"neither", []string{"proc/net/tcp", "proc/net/tcp6"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := copyFixture(t)
			for _, p := range tc.missing {
				if err := os.Remove(filepath.Join(root, p)); err != nil {
					t.Fatal(err)
				}
			}
			if c, l := runAt(root, 1793404800); c != 0 {
				t.Fatal(c, l)
			}
			idx := index(t, root)
			if tc.column > 0 && len(idx.Unavailable) != 0 {
				t.Fatal(idx.Unavailable)
			}
			if tc.column == 0 {
				if len(idx.Unavailable) != 1 {
					t.Fatal(idx.Unavailable)
				}
				u := idx.Unavailable[0]
				if u.Group != "tcp" || u.Column != nil || u.Reason != "unsupported" {
					t.Fatal(u)
				}
			}
			b, err := os.ReadFile(filepath.Join(root, "var/lib/hostdiary/public/data/1m/2026-10-31/tcp.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			lines := strings.Split(strings.TrimSpace(string(b)), "\n")
			var row []any
			if err = json.Unmarshal([]byte(lines[len(lines)-1]), &row); err != nil || len(row) != 15 {
				t.Fatal(row, err)
			}
			if tc.column > 0 {
				if row[tc.column] != float64(1) {
					t.Fatal(row)
				}
			} else {
				for _, v := range row[1:] {
					if v != nil {
						t.Fatal(row)
					}
				}
			}
		})
	}
}

func TestUnavailableIndexVocabulary(t *testing.T) {
	root := copyFixture(t)
	for _, p := range []string{"proc/uptime", "proc/sys/kernel/random/boot_id", "proc/net/tcp6"} {
		if e := os.Remove(filepath.Join(root, p)); e != nil {
			t.Fatal(e)
		}
	}
	if c, l := runAt(root, 1793404800); c != 0 {
		t.Fatal(c, l)
	}
	idx := index(t, root)
	got := []string{}
	for _, u := range idx.Unavailable {
		if u.Column != nil || u.Reason != "unsupported" {
			t.Fatal(u)
		}
		got = append(got, u.Group)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, []string{"cpu", "diskio", "net"}) {
		t.Fatal(idx.Unavailable)
	}
}

func TestLongCommConfigWritesNothing(t *testing.T) {
	root := copyFixture(t)
	write(t, root, "etc/hostdiary/config.json", `{"apps":{"worker":["abcdefghijklmnop"]}}`)
	if c, l := runAt(root, 1793404800); c != 2 || !strings.Contains(l, "comm is truncated to 15 bytes") {
		t.Fatal(c, l)
	}
	if _, e := os.Stat(filepath.Join(root, "var")); !os.IsNotExist(e) {
		t.Fatal("invalid config wrote state")
	}
}
