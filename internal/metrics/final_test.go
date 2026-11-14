package metrics

import (
	"reflect"
	"strings"
	"testing"

	"hostdiary/internal/config"
	"hostdiary/internal/procfs"
	"hostdiary/internal/store"
)

func TestNormalizeUnavailable(t *testing.T) {
	col := "rss"
	other := "used"
	count := "count"
	input := []store.Unavailable{{Group: "z", Column: &col, Reason: "error"}, {Group: "mem", Column: &other, Reason: "unsupported"}, {Group: "mem", Reason: "error"}, {Group: "a", Column: &col, Reason: "unsupported"}, {Group: "z", Column: &col, Reason: "error"}, {Group: "a", Column: &col, Reason: "error"}, {Group: "z", Column: &count, Reason: "error"}}
	want := []store.Unavailable{{Group: "a", Column: &col, Reason: "error"}, {Group: "a", Column: &col, Reason: "unsupported"}, {Group: "mem", Reason: "error"}, {Group: "z", Column: &count, Reason: "error"}, {Group: "z", Column: &col, Reason: "error"}}
	if got := normalizeUnavailable(input); !reflect.DeepEqual(got, want) {
		t.Fatal(got)
	}
	if input[0].Group != "z" {
		t.Fatal("mutated input")
	}
}

func TestCPUAndTCPDescriptionsExplainMeasurements(t *testing.T) {
	rows, _ := BuildRows(Observation{}, procfs.Snapshot{}, config.Config{}, 12345)
	for _, row := range rows {
		if row.Group.Kind != "cpu" && row.Group.Kind != "tcp" {
			continue
		}
		for i, desc := range row.Group.Descriptions[1:] {
			if row.Group.Kind == "tcp" && i >= len(procfs.TCPColumns) {
				continue // Segment rates added after the existing socket-state columns.
			}
			if desc == row.Group.Columns[i+1] {
				t.Fatal(desc)
			}
			if row.Group.Kind == "cpu" {
				for _, term := range []string{"all logical CPUs", "previous collection", "100"} {
					if !strings.Contains(desc, term) {
						t.Fatal(desc)
					}
				}
			} else if !strings.Contains(desc, "number of TCP") {
				t.Fatal(desc)
			}
		}
		if row.Group.Kind == "cpu" {
			for i, term := range map[int]string{1: "guest", 4: "negative", 7: "hypervisor"} {
				if !strings.Contains(row.Group.Descriptions[i], term) {
					t.Fatal(row.Group.Descriptions)
				}
			}
		}
		if row.Group.Kind == "tcp" && !strings.Contains(row.Group.Descriptions[12], "1-11") {
			t.Fatal(row.Group.Descriptions)
		}
	}
}
