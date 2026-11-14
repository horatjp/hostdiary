package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Descriptions align with the final header, using columns when metadata is absent
// or has an incompatible length; previous index metadata also remains usable.
func TestIndexDescriptionsFollowLatestHeader(t *testing.T) {
	for _, mode := range []string{"current", "previous", "short", "absent"} {
		t.Run(mode, func(t *testing.T) {
			public := t.TempDir()
			at := int64(1793404800)
			old := testGroup()
			cur := old
			cur.Columns = []string{"time", "user", "idle"}
			cur.Units = []string{"s", "%", "%"}
			cur.Descriptions = []string{"collection time", "current user explanation", "current idle explanation"}
			if e := AppendGroup(public, old, at-86400, []any{at - 86400, 10}); e != nil {
				t.Fatal(e)
			}
			if e := AppendGroup(public, cur, at, []any{at, 20, 80}); e != nil {
				t.Fatal(e)
			}
			input := []Group{cur}
			want := cur.Descriptions
			switch mode {
			case "previous":
				if e := WriteIndex(public, Index{GeneratedAt: at, Groups: input}); e != nil {
					t.Fatal(e)
				}
				input = nil
			case "short":
				input = []Group{old}
				want = cur.Columns
			case "absent":
				input = nil
				want = cur.Columns
			}
			if e := WriteIndex(public, Index{GeneratedAt: at, Groups: input}); e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(filepath.Join(public, "index.json"))
			if e != nil {
				t.Fatal(e)
			}
			var idx Index
			if e := json.Unmarshal(b, &idx); e != nil {
				t.Fatal(e)
			}
			if len(idx.Groups) != 1 || !reflect.DeepEqual(idx.Groups[0].Columns, cur.Columns) || !reflect.DeepEqual(idx.Groups[0].Descriptions, want) {
				t.Fatal(idx.Groups, want)
			}
		})
	}
}
