package store

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

func TestGroupIDContract(t *testing.T) {
	if GroupID("cpu", "cpu") != "cpu" {
		t.Fatal("global ID")
	}
	if GroupID("psi", "psi") != "psi" {
		t.Fatal("PSI must use the host-wide singleton ID")
	}
	for _, name := range []string{"eth0", "A /日本語)", strings.Repeat("x", 100)} {
		id := GroupID("net", name)
		if id != GroupID("net", name) || !regexp.MustCompile(`^[a-z0-9_-]+$`).MatchString(id) || len(id) > 45 {
			t.Fatal(id)
		}
	}
	if GroupID("net", "A") == GroupID("net", "a") {
		t.Fatal("collision")
	}
}
func TestExpiredDaysContract(t *testing.T) {
	names := []string{"2026-10-01", "2026-10-02", "2026-10-03", "2026-10-04", "2026-10-05", "2026-10-06", "2026-10-07", "2026-10-08", "2099-01-01", "notes", "2026-02-30"}
	if got := ExpiredDays(names, 8); !reflect.DeepEqual(got, []string{"2026-10-01"}) {
		t.Fatal(got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("keep=0 must reject")
		}
	}()
	ExpiredDays(names, 0)
}
func TestAppendLineContract(t *testing.T) {
	for _, start := range []string{"", "complete\ncut", "cut", strings.Repeat("x", 20000)} {
		t.Run(start[:min(8, len(start))], func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "data")
			os.WriteFile(p, []byte(start), 0644)
			if err := AppendLine(p, []byte("new")); err != nil {
				t.Fatal(err)
			}
			b, _ := os.ReadFile(p)
			want := "new\n"
			if strings.HasPrefix(start, "complete\n") {
				want = "complete\n" + want
			}
			if string(b) != want {
				t.Fatalf("got %q", b)
			}
		})
	}
}
