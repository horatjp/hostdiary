package collect

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestLongUptimeKeepsCurrentBoot(t *testing.T) {
	root := copyFixture(t)
	at := int64(1793404800)
	const uptime = 20 * 86400
	write(t, root, "proc/uptime", "1728000 0\n")
	for _, offset := range []int64{0, 60} {
		if c, l := runAt(root, at+offset); c != 0 {
			t.Fatal(c, l)
		}
		if got := index(t, root).Boots; !reflect.DeepEqual(got, []int64{at - uptime}) {
			t.Fatal(got)
		}
	}
	write(t, root, "proc/sys/kernel/random/boot_id", "new-fixture-boot\n")
	write(t, root, "proc/uptime", "10 0\n")
	if c, l := runAt(root, at+120); c != 0 {
		t.Fatal(c, l)
	}
	if got := index(t, root).Boots; !reflect.DeepEqual(got, []int64{at - uptime, at + 110}) {
		t.Fatal("past boot inside retention window lost", got)
	}
}

func TestIndexFileFailureStillPublishesOtherGroups(t *testing.T) {
	root := copyFixture(t)
	at := int64(1793404800)
	if c, l := runAt(root, at); c != 0 {
		t.Fatal(c, l)
	}
	path := filepath.Join(root, "var/lib/hostdiary/public/data/1m/2026-10-31/broken.jsonl")
	if e := os.WriteFile(path, []byte(strings.Repeat("x", 1024*1024+1)+"\n"), 0644); e != nil {
		t.Fatal(e)
	}
	if c, l := runAt(root, at+60); c != 1 || !strings.Contains(l, "broken.jsonl") {
		t.Fatal(c, l)
	}
	idx := index(t, root)
	if idx.GeneratedAt != at+60 || len(idx.Groups) == 0 {
		t.Fatal(idx)
	}
}
