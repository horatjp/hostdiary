package collect

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"hostdiary/internal/store"
)

func TestCollectWritesSummaryAndIndexPointers(t *testing.T) {
	root := copyFixture(t)
	at := int64(1793404800)
	if code, log := runAt(root, at); code != 0 {
		t.Fatal(code, log)
	}
	idx := index(t, root)
	if idx.Guide != "llms.txt" || idx.Summary != "summary.json" {
		t.Fatalf("index pointers: %q %q", idx.Guide, idx.Summary)
	}
	public := filepath.Join(root, "var/lib/hostdiary/public")
	b, e := os.ReadFile(filepath.Join(public, "summary.json"))
	if e != nil {
		t.Fatal(e)
	}
	var s store.Summary
	if e := json.Unmarshal(b, &s); e != nil {
		t.Fatal(e)
	}
	if s.GeneratedAt != at || s.Host != "fixture-host" || s.Version != Version || len(s.Days) != 1 || s.Days[0].Status != "partial" || len(s.Days[0].Groups) == 0 {
		t.Fatalf("summary: %s", b)
	}
	if _, e := os.Stat(filepath.Join(public, "llms.txt")); e != nil {
		t.Fatal("guide is published with the assets:", e)
	}
}

func TestCollectReportsSummaryFailure(t *testing.T) {
	root := copyFixture(t)
	public := filepath.Join(root, "var/lib/hostdiary/public")
	// A directory in place of summary.json makes the atomic rename fail.
	if e := os.MkdirAll(filepath.Join(public, "summary.json", "x"), 0755); e != nil {
		t.Fatal(e)
	}
	code, log := runAt(root, 1793404800)
	if code != 1 || !strings.Contains(log, "summary: ") {
		t.Fatal(code, log)
	}
	if idx := index(t, root); idx.GeneratedAt != 1793404800 {
		t.Fatal("index is still written when the summary fails")
	}
}
