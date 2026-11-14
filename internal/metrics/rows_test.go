package metrics

import (
	"hostdiary/internal/config"
	"hostdiary/internal/procfs"
	"hostdiary/internal/store"
	"reflect"
	"testing"
)

func TestRowsContract(t *testing.T) {
	s := procfs.Snapshot{BootID: "a", Uptime: 1060, UptimeValid: true, CPUValid: true, CPU: [8]uint64{20, 20, 20, 20, 20, 20, 20, 20}, CPUs: 2, LoadValid: true, Load: [3]float64{1, 2, 3}, Mem: map[string]uint64{"MemTotal": 100, "MemAvailable": 40, "Buffers": 10, "Cached": 20, "SwapTotal": 50, "SwapFree": 30}, Net: map[string][]uint64{"eth0": {700, 1400}}, DiskIO: map[string][]uint64{"sda": {160, 320, 70, 140, 900}}, Disks: map[string]procfs.FS{"/": {Used: 100, Avail: 200}}, ProcessesValid: true, Processes: []procfs.Process{{Comm: "worker", State: 'S', RSS: 4096}, {Comm: "worker", State: 'Z', RSS: 4096}, {Comm: "idle", State: 'I'}}, TCPValid: true}
	prev := Observation{BootID: "a", Uptime: 1000, UptimeValid: true, CPUValid: true, CPU: [8]uint64{10, 10, 10, 10, 10, 10, 10, 10}, Net: map[string][]uint64{"eth0": {100, 200}}, DiskIO: map[string][]uint64{"sda": {100, 200, 10, 20, 300}}}
	rows, missing := BuildRows(prev, s, config.Config{Apps: map[string][]string{"worker": {"worker", "worker"}}}, 12345)
	byKind := map[string][]any{}
	for _, r := range rows {
		byKind[r.Group.Kind] = r.Values
		if len(r.Group.Columns) != len(r.Values) || len(r.Group.Units) != len(r.Values) || len(r.Group.Descriptions) != len(r.Values) {
			t.Fatal(r)
		}
	}
	checks := []struct {
		kind string
		col  int
		want any
	}{{"net", 1, float64(10)}, {"net", 2, float64(20)}, {"diskio", 1, float64(512)}, {"diskio", 5, float64(1)}, {"cpu", 1, float64(12.5)}, {"mem", 3, uint64(60)}, {"swap", 2, uint64(20)}, {"app", 1, uint64(1)}, {"app", 2, uint64(4096)}, {"procs", 1, uint64(3)}, {"procs", 7, uint64(1)}, {"disk", 3, nil}}
	for _, c := range checks {
		if got := byKind[c.kind][c.col]; got != c.want {
			t.Fatalf("%s[%d]=%v want %v", c.kind, c.col, got, c.want)
		}
	}
	iu, it := "inodes_used", "inodes_total"
	wantMissing := []store.Unavailable{{Group: store.GroupID("disk", "/"), Column: &it, Reason: "unsupported"}, {Group: store.GroupID("disk", "/"), Column: &iu, Reason: "unsupported"}}
	if !reflect.DeepEqual(missing, wantMissing) {
		t.Fatal(missing)
	}
	// Wall-clock timestamps affect only the first column, never the rate denominator.
	rows, _ = BuildRows(prev, s, config.Config{}, 99999)
	for _, r := range rows {
		if r.Group.Kind == "net" && r.Values[1] != float64(10) {
			t.Fatal(r)
		}
	}
	s.BootID = "b"
	rows, _ = BuildRows(prev, s, config.Config{}, 12346)
	for _, r := range rows {
		if r.Group.Kind == "net" || r.Group.Kind == "cpu" || r.Group.Kind == "diskio" {
			for _, v := range r.Values[1:] {
				if v != nil {
					t.Fatal(r)
				}
			}
		}
	}
}

func TestDependentUnavailableReasons(t *testing.T) {
	s := procfs.Snapshot{Unavailable: []store.Unavailable{{Group: "mem", Reason: "error"}, {Group: "procs", Reason: "unsupported"}}}
	_, missing := BuildRows(Observation{}, s, config.Config{Apps: map[string][]string{"worker": {"worker"}}}, 12345)
	for _, u := range missing {
		if (u.Group == "mem" || u.Group == "swap") && u.Reason != "error" {
			t.Fatal(u)
		}
		if u.Group == store.GroupID("app", "worker") && u.Reason != "unsupported" {
			t.Fatal(u)
		}
	}
}
