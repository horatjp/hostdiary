package collect

import (
	"path/filepath"
	"reflect"
	"testing"

	"hostdiary/internal/store"
)

// Current descriptions must survive scanning an older day's shorter header first.
func TestDescriptionsSurviveRetainedOlderHeaders(t *testing.T) {
	root := copyFixture(t)
	at := int64(1793404800)
	if code, log := runAt(root, at); code != 0 {
		t.Fatal(code, log)
	}
	expected := map[string]store.Group{}
	oldLengths := map[string]int{"mem": 6, "swap": 3, "tcp": 13, "net": 3, "diskio": 6, "app": 3}
	public := filepath.Join(root, "var/lib/hostdiary/public")
	for _, g := range index(t, root).Groups {
		length, ok := oldLengths[g.Kind]
		if !ok {
			continue
		}
		expected[g.ID] = g
		old := g
		old.Columns = old.Columns[:length]
		old.Units = old.Units[:length]
		old.Descriptions = old.Descriptions[:length]
		row := make([]any, length)
		row[0] = at - 86400
		if e := store.AppendGroup(public, old, at-86400, row); e != nil {
			t.Fatal(e)
		}
	}
	if len(expected) != len(oldLengths) {
		t.Fatal("fixture does not exercise all extended groups", len(expected))
	}
	write(t, root, "proc/uptime", "1060 1900\n")
	if code, log := runAt(root, at+60); code != 0 {
		t.Fatal(code, log)
	}
	for _, g := range index(t, root).Groups {
		want, ok := expected[g.ID]
		if !ok {
			continue
		}
		if len(g.Days) != 2 || !reflect.DeepEqual(g.Columns, want.Columns) || !reflect.DeepEqual(g.Descriptions, want.Descriptions) {
			t.Errorf("%s: columns=%v descriptions=%v; want current descriptions=%v", g.Kind, g.Columns, g.Descriptions, want.Descriptions)
		}
		delete(expected, g.ID)
	}
	if len(expected) != 0 {
		t.Fatal("missing groups", expected)
	}
}
