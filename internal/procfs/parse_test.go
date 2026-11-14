package procfs

import (
	"os"
	"path/filepath"
	"testing"
)

func fixture(t *testing.T, path string) string {
	t.Helper()
	b, e := os.ReadFile(filepath.Join("../../testdata/proc", path))
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}
func TestParsers(t *testing.T) {
	t.Run("stat", func(t *testing.T) {
		v, n, e := ParseStat(fixture(t, "stat"))
		if e != nil || n != 2 || v[3] != 10 || v[7] != 400 {
			t.Fatal(v, n, e)
		}
	})
	t.Run("loadavg", func(t *testing.T) {
		v, e := ParseLoad(fixture(t, "loadavg"))
		if e != nil || v[2] != .75 {
			t.Fatal(v, e)
		}
	})
	t.Run("uptime", func(t *testing.T) {
		v, e := ParseUptime(fixture(t, "uptime"))
		if e != nil || v != 1000 {
			t.Fatal(v, e)
		}
	})
	t.Run("meminfo", func(t *testing.T) {
		v, e := ParseMem(fixture(t, "meminfo"))
		if e != nil || v["MemTotal"] != 8192*1024 {
			t.Fatal(v, e)
		}
	})
	t.Run("net", func(t *testing.T) {
		v, e := ParseNet(fixture(t, "net/dev"))
		if e != nil || v["eth0"][1] != 2000 {
			t.Fatal(v, e)
		}
	})
	t.Run("diskstats", func(t *testing.T) {
		v, e := ParseDiskStats(fixture(t, "diskstats"))
		if e != nil || v["sda"][0] != 100 || v["sda"][4] != 300 {
			t.Fatal(v, e)
		}
	})
	t.Run("mountinfo", func(t *testing.T) {
		v, e := ParseMounts(fixture(t, "self/mountinfo"))
		if e != nil {
			t.Fatal(e)
		}
		chosen := SelectMounts(v)
		if len(chosen) != 2 {
			t.Fatal(chosen)
		}
		for _, m := range chosen {
			if m.Point != "/" && m.Point != "/space disk" {
				t.Fatal(m)
			}
		}
	})
	t.Run("pid", func(t *testing.T) {
		v, e := ParsePIDStat(fixture(t, "101/stat"), 4096)
		if e != nil || v.Comm != "worker (one)" || v.RSS != 40960 || v.State != 'S' {
			t.Fatal(v, e)
		}
	})
	t.Run("tcp", func(t *testing.T) {
		v, e := ParseTCP(fixture(t, "net/tcp"))
		if e != nil || v[0] != 1 || v[9] != 1 {
			t.Fatal(v, e)
		}
	})
	t.Run("tcp6", func(t *testing.T) {
		v, e := ParseTCP(fixture(t, "net/tcp6"))
		if e != nil || v[5] != 1 {
			t.Fatal(v, e)
		}
	})
}
func TestMalformed(t *testing.T) {
	for _, tc := range []struct {
		name  string
		parse func() error
	}{
		{"stat", func() error { _, _, e := ParseStat("cpu 1"); return e }},
		{"load", func() error { _, e := ParseLoad("NaN 0 1"); return e }},
		{"uptime", func() error { _, e := ParseUptime("-1"); return e }},
		{"mem", func() error { _, e := ParseMem("MemTotal: x kB"); return e }},
		{"net", func() error { _, e := ParseNet("eth0: 1"); return e }},
		{"disk", func() error { _, e := ParseDiskStats("8 0 sda 1"); return e }},
		{"mount", func() error { _, e := ParseMounts("invalid"); return e }},
		{"pid", func() error { _, e := ParsePIDStat("1 (bad) S", 4096); return e }},
		{"tcp", func() error { _, e := ParseTCP("0: x x XX"); return e }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.parse() == nil {
				t.Fatal("accepted malformed input")
			}
		})
	}
}
