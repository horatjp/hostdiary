package procfs

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"hostdiary/internal/config"
)

func TestParsePSIContract(t *testing.T) {
	for _, tc := range []struct {
		text         string
		some, full   uint64
		hasFull, bad bool
	}{
		{"some avg10=1.23 avg60=2.34 avg300=3.45 total=101\nfull avg10=4.56 total=202", 101, 202, true, false},
		{"some total=0", 0, 0, false, false},
		{"full total=1", 0, 0, false, true},
		{"some avg10=1", 0, 0, false, true},
		{"some total=bad", 0, 0, false, true},
		{"some total=-1", 0, 0, false, true},
		{"some total=1\nfull total=x", 0, 0, false, true},
	} {
		t.Run(tc.text, func(t *testing.T) {
			s, f, e := ParsePSI(tc.text)
			if (e != nil) != tc.bad {
				t.Fatal(s, f, e)
			}
			if !tc.bad && (s == nil || *s != tc.some || (f != nil) != tc.hasFull || f != nil && *f != tc.full) {
				t.Fatal(s, f)
			}
		})
	}
}

func TestParseVMStatContract(t *testing.T) {
	v := ParseVMStat("pswpin 11\npswpout 22\noom_kill bad\nother -1\n")
	if !reflect.DeepEqual(v, map[string]uint64{"pswpin": 11, "pswpout": 22}) {
		t.Fatal(v)
	}
	if got := ParseVMStat("oom_kill 0"); len(got) != 1 || got["oom_kill"] != 0 {
		t.Fatal(got)
	}
}

func TestParseSNMPTCPContract(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		bad        bool
	}{
		{"fixture", fixture(t, "net/snmp"), false},
		{"reordered", "Ip: Forwarding\nIp: 1\nTcp: RetransSegs MaxConn OutSegs\nTcp: 202 -1 101", false},
		{"missing row", "Tcp: OutSegs RetransSegs", true},
		{"short row", "Tcp: OutSegs RetransSegs\nTcp: 101", true},
		{"missing column", "Tcp: OutSegs MaxConn\nTcp: 101 -1", true},
		{"negative", "Tcp: OutSegs RetransSegs\nTcp: -1 202", true},
		{"invalid", "Tcp: OutSegs RetransSegs\nTcp: 101 x", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v, e := ParseSNMPTCP(tc.text)
			if (e != nil) != tc.bad || !tc.bad && !reflect.DeepEqual(v, []uint64{101, 202}) {
				t.Fatal(v, e)
			}
		})
	}
}

func TestExtraFixtureColumns(t *testing.T) {
	n, e := ParseNet(fixture(t, "net/dev"))
	if e != nil || !reflect.DeepEqual(n["eth0"], []uint64{1000, 2000, 12, 13, 22, 23}) {
		t.Fatal(n, e)
	}
	d, e := ParseDiskStats(fixture(t, "diskstats"))
	if e != nil || !reflect.DeepEqual(d["sda"], []uint64{100, 200, 10, 20, 300, 14, 24}) {
		t.Fatal(d, e)
	}
	p, e := ParsePIDStat("101 (worker (one)) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 10051 19 10", 4096)
	if e != nil || p.Start != 10051 || p.RSS != 40960 || p.Comm != "worker (one)" {
		t.Fatal(p, e)
	}
	_, e = ParsePIDStat("101 (worker) S 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 -1 19 10", 4096)
	if e == nil {
		t.Fatal("negative start accepted")
	}
}

func TestExtraSnapshotAvailability(t *testing.T) {
	for _, tc := range []struct {
		name, remove, broken, text string
		want                       []string
		errors                     int
	}{
		{name: "all"},
		{name: "cpu full unused", broken: "pressure/cpu", text: "some total=101\nfull total=999\n"},
		{name: "pressure directory", remove: "pressure", want: []string{"psi:*:unsupported"}},
		{name: "memory pressure", remove: "pressure/memory", want: []string{"psi:memory_full:unsupported", "psi:memory_some:unsupported"}},
		{name: "full absent", broken: "pressure/io", text: "some total=1\n", want: []string{"psi:io_full:unsupported"}},
		{name: "broken pressure", broken: "pressure/cpu", text: "bad", want: []string{"psi:cpu_some:error"}, errors: 1},
		{name: "vmstat missing", remove: "vmstat", want: []string{"mem:oom_kills:unsupported", "swap:in:unsupported", "swap:out:unsupported"}},
		{name: "oom absent", broken: "vmstat", text: "pswpin 11\npswpout 22\n", want: []string{"mem:oom_kills:unsupported"}},
		{name: "oom malformed", broken: "vmstat", text: "pswpin 11\npswpout 22\noom_kill x\n", want: []string{"mem:oom_kills:error"}, errors: 1},
		{name: "snmp absent", remove: "net/snmp", want: []string{"tcp:out_segs:unsupported", "tcp:retrans_segs:unsupported"}},
		{name: "snmp malformed", broken: "net/snmp", text: "bad", want: []string{"tcp:out_segs:error", "tcp:retrans_segs:error"}, errors: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := snapshotRoot(t)
			if tc.remove != "" {
				if e := os.RemoveAll(filepath.Join(root, "proc", tc.remove)); e != nil {
					t.Fatal(e)
				}
			}
			if tc.broken != "" {
				if e := os.WriteFile(filepath.Join(root, "proc", tc.broken), []byte(tc.text), 0644); e != nil {
					t.Fatal(e)
				}
			}
			s := ReadSnapshot(root, config.Config{Disk: config.Filter{Exclude: []string{"/", "/space disk"}}})
			got := []string{}
			for _, u := range s.Unavailable {
				col := "*"
				if u.Column != nil {
					col = *u.Column
				}
				got = append(got, u.Group+":"+col+":"+u.Reason)
			}
			want := append([]string{}, tc.want...)
			sort.Strings(got)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) || len(s.Errors) != tc.errors {
				t.Fatal(got, want, s.Errors)
			}
			for _, e := range s.Errors {
				if strings.Contains(e.Error(), root) {
					t.Fatal("fixture root leaked", e)
				}
			}
			if (tc.name == "all" || tc.name == "cpu full unused") && (len(s.PSI) != 5 || s.PSI["cpu_some"] != 101 || len(s.VMStat) != 3 || !reflect.DeepEqual(s.TCPSegs, []uint64{101, 202})) {
				t.Fatal(s)
			}
		})
	}
}
