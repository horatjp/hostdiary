package metrics

import (
	"math"
	"os"
	"strings"
	"testing"
)

func TestExtraPerCounterPresenceAndDecrease(t *testing.T) {
	s := extraSnapshot()
	p := Observe(s)
	p.Uptime -= 60
	p.PSI = map[string]uint64{"cpu_some": 720000001, "memory_some": 0}
	p.VMStat = map[string]uint64{"pswpin": 121, "pswpout": 0, "oom_kill": 6}
	p.TCPSegs = []uint64{601, 0}
	p.Net = map[string][]uint64{"eth0": {0, 0, 181, 0, 0, 0}}
	p.DiskIO = map[string][]uint64{"sda": {0, 0, 0, 0, 0, 361, 0}}
	r := extraRows(p, s)
	for _, tc := range []struct{ kind, col string }{{"psi", "cpu_some"}, {"psi", "memory_full"}, {"swap", "in"}, {"mem", "oom_kills"}, {"tcp", "out_segs"}, {"net", "rx_errors"}, {"diskio", "read_time"}} {
		assertColumn(t, r, tc.kind, tc.col, nil)
	}
	assertColumn(t, r, "psi", "memory_some", float64(10))
	assertColumn(t, r, "tcp", "retrans_segs", float64(2))
	assertColumn(t, r, "net", "rx_drops", float64(4))
	assertColumn(t, r, "diskio", "write_time", float64(7))
	assertColumn(t, r, "swap", "out", float64(3*os.Getpagesize()))
}

func TestExtraPartialCounterArraysPreservePresentPositions(t *testing.T) {
	s := extraSnapshot()
	p := Observe(s)
	p.Uptime -= 60
	p.Net = map[string][]uint64{"eth0": {0, 0, 0}}
	p.DiskIO = map[string][]uint64{"sda": {0, 0, 0, 0, 0, 0}}
	p.TCPSegs = []uint64{0}
	r := extraRows(p, s)
	assertColumn(t, r, "net", "rx_errors", float64(3))
	assertColumn(t, r, "net", "rx_drops", nil)
	assertColumn(t, r, "diskio", "read_time", float64(6))
	assertColumn(t, r, "diskio", "write_time", nil)
	assertColumn(t, r, "tcp", "out_segs", float64(10))
	assertColumn(t, r, "tcp", "retrans_segs", nil)
}

func TestCentisecondsValidation(t *testing.T) {
	for _, tc := range []struct {
		u         float64
		valid, ok bool
		want      uint64
	}{
		{100.50, true, true, 10050}, {100.51, true, true, 10051}, {0, true, true, 0},
		{100.50, false, false, 0}, {-1, true, false, 0}, {math.NaN(), true, false, 0}, {math.Inf(1), true, false, 0}, {math.MaxFloat64, true, false, 0},
	} {
		got, ok := centiseconds(tc.u, tc.valid)
		if ok != tc.ok || got != tc.want {
			t.Fatal(tc, got, ok)
		}
	}
}

func TestExtraDescriptions(t *testing.T) {
	r := extraRows(Observe(extraSnapshot()), extraSnapshot())
	for _, tc := range []struct {
		kind, col string
		terms     []string
	}{
		{"mem", "oom_kills", []string{"cgroup", "mem.available", "samples", "no interval upper limit"}},
		{"app", "started", []string{"USER_HZ=100", "exited", "samples"}},
		{"diskio", "read_time", []string{"1000", "r_await", "Not application response time", "null patterns", "rounding"}},
		{"diskio", "write_time", []string{"w_await", "write_time / write_ops"}},
		{"tcp", "out_segs", []string{"pure ACKs", "excluding retransmissions", "IPv4 + IPv6"}},
		{"tcp", "retrans_segs", []string{"not packet loss rate", "matching null patterns"}},
		{"net", "rx_drops", []string{"no protocol handler", "does not identify the cause"}},
		{"net", "tx_drops", []string{"NIC driver", "virtual NICs"}},
		{"psi", "memory_full", []string{"all non-idle tasks", "during the interval"}},
		{"psi", "cpu_some", []string{"at least one task (some) was stalled"}},
		{"swap", "in", []string{"bytes swapped in per second"}},
		{"swap", "out", []string{"bytes swapped out per second"}},
	} {
		row := r[tc.kind]
		found := false
		for i, col := range row.Group.Columns {
			if col == tc.col {
				found = true
				for _, term := range tc.terms {
					if !strings.Contains(row.Group.Descriptions[i], term) {
						t.Fatal(tc.kind, col, term, row.Group.Descriptions[i])
					}
				}
			}
		}
		if !found {
			t.Fatal(tc)
		}
	}
}
