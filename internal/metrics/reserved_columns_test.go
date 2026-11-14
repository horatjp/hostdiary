package metrics

import (
	"hostdiary/internal/config"
	"hostdiary/internal/procfs"
	"strings"
	"testing"
)

// All emitted minute schemas must remain valid sources for coarse values.
func TestAllGroupColumnsAvoidCoarseReservedNames(t *testing.T) {
	s := procfs.Snapshot{Net: map[string][]uint64{"fixture": {1, 2}}, DiskIO: map[string][]uint64{"fixture": {1, 2, 3, 4, 5}}, Disks: map[string]procfs.FS{"/fixture": {}}}
	rows, _ := BuildRows(Observation{}, s, config.Config{Apps: map[string][]string{"fixture": {"worker"}}}, 123)
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.Group.Kind] = true
		for _, c := range r.Group.Columns {
			if c == "samples" || c == "last_time" || strings.HasSuffix(c, "_max") {
				t.Fatalf("%s: reserved column %s", r.Group.Kind, c)
			}
		}
	}
	for _, kind := range []string{"cpu", "load", "mem", "swap", "procs", "tcp", "net", "disk", "diskio", "app"} {
		if !seen[kind] {
			t.Fatal("untested group kind", kind)
		}
	}
}
