package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ValidDay accepts only a real calendar day in canonical YYYY-MM-DD form.
func TestValidDay(t *testing.T) {
	for _, tc := range []struct {
		day  string
		want bool
	}{{"2024-02-29", true}, {"2026-02-29", false}, {"2026-11-01", true}, {"2026-1-01", false}, {"2026-11-1", false}, {"2026-00-01", false}, {"2026-11-01.json", false}, {"", false}} {
		if got := ValidDay(tc.day); got != tc.want {
			t.Fatal(tc.day, got)
		}
	}
}

func TestIndexDirsNamesOnlyAndOmission(t *testing.T) {
	public := filepath.Join(t.TempDir(), "public")
	dir := filepath.Join(public, "data/dirs")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"2026-11-01.json", "2026-10-25.json", "2026-02-30.json", "2026-10-24.js", "notes.json"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("not JSON"), 0000); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "2026-10-23.json"), 0755); err != nil {
		t.Fatal(err)
	}
	for pass := 0; pass < 2; pass++ {
		if pass == 1 {
			for _, name := range []string{"2026-11-01.json", "2026-10-25.json"} {
				if err := os.Remove(filepath.Join(dir, name)); err != nil {
					t.Fatal(err)
				}
			}
		}
		if err := WriteIndex(public, Index{Schema: 1}); err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(filepath.Join(public, "index.json"))
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]json.RawMessage
		if err = json.Unmarshal(b, &got); err != nil {
			t.Fatal(err)
		}
		if pass == 1 {
			if _, ok := got["dirs"]; ok {
				t.Fatal(string(b))
			}
			continue
		}
		var dirs struct {
			Days              []string
			Path, Description string
		}
		if err = json.Unmarshal(got["dirs"], &dirs); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(dirs.Days, []string{"2026-10-25", "2026-11-01"}) || dirs.Path != "data/dirs/{day}.json" {
			t.Fatal(dirs)
		}
		for _, s := range []string{"Parent and child entries overlap", "absence does not mean zero", "deleted-but-open files", "errors counts entries skipped", "escaped", "When complete=false, total and entries are lower bounds; a filesystem whose mount point itself could not be read has total 0 and no entries", "Blocks shared by hard links are attributed to the first path visited in lexical order."} {
			if !strings.Contains(dirs.Description, s) {
				t.Fatal(dirs.Description)
			}
		}
	}
}
