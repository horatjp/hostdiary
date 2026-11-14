package metrics

import (
	"encoding/json"
	"math"
	"os"
	"reflect"
	"testing"

	"hostdiary/internal/config"
	"hostdiary/internal/procfs"
)

func TestDeltaContract(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prev, cur Counter
		want      *uint64
	}{
		{"increase", Counter{2, "a", true}, Counter{5, "a", true}, uintPtr(3)},
		{"zero baseline", Counter{0, "a", true}, Counter{5, "a", true}, uintPtr(5)},
		{"equal", Counter{5, "a", true}, Counter{5, "a", true}, uintPtr(0)},
		{"absent", Counter{}, Counter{5, "a", true}, nil},
		{"current absent", Counter{2, "a", true}, Counter{}, nil},
		{"reset", Counter{5, "a", true}, Counter{2, "a", true}, nil},
		{"boot", Counter{2, "a", true}, Counter{5, "b", true}, nil},
		{"empty boot", Counter{2, "", true}, Counter{5, "", true}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := delta(tc.prev, tc.cur); !reflect.DeepEqual(got, tc.want) {
				t.Fatal(got, tc.want)
			}
		})
	}
}
func uintPtr(n uint64) *uint64 { return &n }

func TestStartedContract(t *testing.T) {
	p := []procfs.Process{{Comm: "w", State: 'S', Start: 10050}, {Comm: "w", State: 'R', Start: 10051}, {Comm: "w", State: 'S', Start: 16050}, {Comm: "w", State: 'S', Start: 16051}, {Comm: "w", State: 'Z', Start: 10052}, {Comm: "other", State: 'S', Start: 10053}}
	for _, tc := range []struct {
		name      string
		prev, cur uint64
		boot      bool
		want      *uint64
	}{
		{"boundaries", 10050, 16050, true, uintPtr(2)},
		{"equal", 10050, 10050, true, nil},
		{"backward", 16050, 10050, true, nil},
		{"boot", 10050, 16050, false, nil},
		{"long interval", 10050, 100000, true, uintPtr(3)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := started(p, []string{"w", "w"}, tc.prev, tc.cur, tc.boot); !reflect.DeepEqual(got, tc.want) {
				t.Fatal(got, tc.want)
			}
		})
	}
}

func extraSnapshot() procfs.Snapshot {
	return procfs.Snapshot{BootID: "a", Uptime: 160.50, UptimeValid: true, TCPValid: true, ProcessesValid: true,
		PSI:    map[string]uint64{"cpu_some": 720000000, "memory_some": 6000000, "memory_full": 3000000, "io_some": 12000000, "io_full": 9000000},
		VMStat: map[string]uint64{"pswpin": 120, "pswpout": 180, "oom_kill": 5}, TCPSegs: []uint64{600, 120},
		Net: map[string][]uint64{"eth0": {600, 1200, 180, 240, 300, 360}}, DiskIO: map[string][]uint64{"sda": {600, 1200, 180, 240, 300, 360, 420}},
		Processes: []procfs.Process{{Comm: "w", State: 'S', Start: 10050}, {Comm: "w", State: 'R', Start: 10051}, {Comm: "w", State: 'S', Start: 16050}, {Comm: "w", State: 'S', Start: 16051}, {Comm: "w", State: 'Z', Start: 10052}}}
}
func extraRows(prev Observation, s procfs.Snapshot) map[string]Row {
	rows, _ := BuildRows(prev, s, config.Config{Apps: map[string][]string{"worker": {"w"}}}, 12345)
	out := map[string]Row{}
	for _, r := range rows {
		out[r.Group.Kind] = r
	}
	return out
}
func assertColumn(t *testing.T, rows map[string]Row, kind, col string, want any) {
	t.Helper()
	r := rows[kind]
	for i, c := range r.Group.Columns {
		if c == col {
			if !reflect.DeepEqual(r.Values[i], want) {
				t.Fatalf("%s.%s = %v (%T), want %v (%T)", kind, col, r.Values[i], r.Values[i], want, want)
			}
			return
		}
	}
	t.Fatalf("missing %s.%s", kind, col)
}

func TestExtraRowsCalculationsAndSchema(t *testing.T) {
	s := extraSnapshot()
	p := Observe(s)
	p.Uptime = 100.50
	p.PSI = map[string]uint64{"cpu_some": 0, "memory_some": 0, "memory_full": 0, "io_some": 0, "io_full": 0}
	p.VMStat = map[string]uint64{"pswpin": 0, "pswpout": 0, "oom_kill": 2}
	p.TCPSegs = []uint64{0, 0}
	p.Net = map[string][]uint64{"eth0": {0, 0, 0, 0, 0, 0}}
	p.DiskIO = map[string][]uint64{"sda": {0, 0, 0, 0, 0, 0, 0}}
	r := extraRows(p, s)
	for _, tc := range []struct {
		kind, col string
		want      any
	}{
		{"psi", "cpu_some", float64(1200)}, {"psi", "memory_some", float64(10)}, {"psi", "memory_full", float64(5)}, {"psi", "io_some", float64(20)}, {"psi", "io_full", float64(15)},
		{"diskio", "read_time", float64(6)}, {"diskio", "write_time", float64(7)}, {"swap", "in", float64(2 * os.Getpagesize())}, {"swap", "out", float64(3 * os.Getpagesize())}, {"mem", "oom_kills", uint64(3)}, {"app", "started", uint64(2)},
		{"tcp", "out_segs", float64(10)}, {"tcp", "retrans_segs", float64(2)}, {"net", "rx_errors", float64(3)}, {"net", "rx_drops", float64(4)}, {"net", "tx_errors", float64(5)}, {"net", "tx_drops", float64(6)},
	} {
		assertColumn(t, r, tc.kind, tc.col, tc.want)
	}
	for _, tc := range []struct {
		kind        string
		cols, units []string
	}{
		{"net", []string{"time", "rx", "tx", "rx_errors", "rx_drops", "tx_errors", "tx_drops"}, []string{"s", "bytes/s", "bytes/s", "count/s", "count/s", "count/s", "count/s"}},
		{"diskio", []string{"time", "read", "write", "read_ops", "write_ops", "busy", "read_time", "write_time"}, []string{"s", "bytes/s", "bytes/s", "count/s", "count/s", "%", "ms/s", "ms/s"}},
		{"swap", []string{"time", "total", "used", "in", "out"}, []string{"s", "bytes", "bytes", "bytes/s", "bytes/s"}},
		{"mem", []string{"time", "total", "available", "used", "buffers", "cached", "oom_kills"}, []string{"s", "bytes", "bytes", "bytes", "bytes", "bytes", "count"}},
		{"app", []string{"time", "count", "rss", "started"}, []string{"s", "count", "bytes", "count"}},
		{"psi", []string{"time", "cpu_some", "memory_some", "memory_full", "io_some", "io_full"}, []string{"s", "%", "%", "%", "%", "%"}},
	} {
		g := r[tc.kind].Group
		if !reflect.DeepEqual(g.Columns, tc.cols) || !reflect.DeepEqual(g.Units, tc.units) {
			t.Fatal(g)
		}
	}
	for _, row := range r {
		if len(row.Values) != len(row.Group.Descriptions) || len(row.Values) != len(row.Group.Columns) || len(row.Values) != len(row.Group.Units) {
			t.Fatal(row)
		}
	}
}

func TestExtraRowsOldStateAndRecovery(t *testing.T) {
	var p Observation
	if e := json.Unmarshal([]byte(`{"boot_id":"a","uptime":100.50,"uptime_valid":true,"net":{"eth0":[1,2]},"diskio":{"sda":[1,2,3,4,5]}}`), &p); e != nil {
		t.Fatal(e)
	}
	s := extraSnapshot()
	rows := extraRows(p, s)
	for _, v := range rows["psi"].Values[1:] {
		if v != nil {
			t.Fatal(rows["psi"])
		}
	}
	for _, tc := range []struct {
		kind, col string
		want      float64
	}{
		{"net", "rx", 9.983}, {"net", "tx", 19.967},
		{"diskio", "read", 5111.467}, {"diskio", "write", 10222.933},
		{"diskio", "read_ops", 2.95}, {"diskio", "write_ops", 3.933}, {"diskio", "busy", 0.492},
	} {
		assertColumn(t, rows, tc.kind, tc.col, tc.want)
	}
	for _, tc := range []struct{ kind, col string }{{"net", "rx_errors"}, {"net", "rx_drops"}, {"net", "tx_errors"}, {"net", "tx_drops"}, {"diskio", "read_time"}, {"diskio", "write_time"}} {
		assertColumn(t, rows, tc.kind, tc.col, nil)
	}
	for _, tc := range []struct{ kind, col string }{{"swap", "in"}, {"swap", "out"}, {"mem", "oom_kills"}, {"tcp", "out_segs"}, {"tcp", "retrans_segs"}} {
		assertColumn(t, rows, tc.kind, tc.col, nil)
	}
	assertColumn(t, rows, "app", "started", uint64(2))
	// State JSON roundtrip preserves presence, including measured zero counters.
	b, e := json.Marshal(Observe(s))
	if e != nil {
		t.Fatal(e)
	}
	if e = json.Unmarshal(b, &p); e != nil {
		t.Fatal(e)
	}
	s.Uptime += 60
	rows = extraRows(p, s)
	assertColumn(t, rows, "psi", "cpu_some", float64(0))
	assertColumn(t, rows, "mem", "oom_kills", uint64(0))
	assertColumn(t, rows, "tcp", "out_segs", float64(0))
	assertColumn(t, rows, "net", "rx_errors", float64(0))
	assertColumn(t, rows, "diskio", "read_time", float64(0))
	s.TCPSegs = nil
	p = Observe(s)
	s.TCPSegs = []uint64{900, 200}
	s.Uptime += 60
	rows = extraRows(p, s)
	assertColumn(t, rows, "tcp", "out_segs", nil)
	p = Observe(s)
	s.Uptime += 60
	rows = extraRows(p, s)
	assertColumn(t, rows, "tcp", "out_segs", float64(0))
}

func TestExtraRowsInvalidIntervals(t *testing.T) {
	for _, tc := range []struct {
		name  string
		dt    float64
		boot  string
		valid bool
		count any
	}{
		{"long", 600, "a", true, uint64(0)},
		{"reboot", 60, "b", true, nil},
		{"empty boot", 60, "", true, nil},
		{"missing uptime", 60, "a", false, uint64(0)},
		{"zero dt", 0, "a", true, uint64(0)},
		{"backward", -1, "a", true, uint64(0)},
		{"nan", math.NaN(), "a", true, uint64(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := extraSnapshot()
			p := Observe(s)
			p.Uptime = s.Uptime - tc.dt
			if tc.name == "long" {
				p.Uptime, s.Uptime = 0.5, 600.5
			}
			p.UptimeValid = tc.valid
			s.BootID = tc.boot
			rows := extraRows(p, s)
			assertColumn(t, rows, "psi", "cpu_some", nil)
			assertColumn(t, rows, "swap", "in", nil)
			assertColumn(t, rows, "tcp", "out_segs", nil)
			assertColumn(t, rows, "mem", "oom_kills", tc.count)
			if tc.name == "long" {
				assertColumn(t, rows, "app", "started", uint64(4))
			} else {
				assertColumn(t, rows, "app", "started", nil)
			}
		})
	}
	s := extraSnapshot()
	p := Observe(s)
	p.Uptime -= 60
	s.TCPValid = false
	rows := extraRows(p, s)
	for _, v := range rows["tcp"].Values[1:] {
		if v != nil {
			t.Fatal(rows["tcp"])
		}
	}
	s = extraSnapshot()
	p = Observe(s)
	p.Uptime -= 60
	s.ProcessesValid = false
	rows = extraRows(p, s)
	assertColumn(t, rows, "app", "started", nil)
}
