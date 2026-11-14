// Package procfs separates Linux text parsing from filesystem reads.
package procfs

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

func uints(fields []string) ([]uint64, error) {
	v := make([]uint64, len(fields))
	for i, s := range fields {
		n, e := strconv.ParseUint(s, 10, 64)
		if e != nil {
			return nil, e
		}
		v[i] = n
	}
	return v, nil
}
func ParseStat(text string) ([8]uint64, int, error) {
	var cpu [8]uint64
	cpus := 0
	seen := false
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if f[0] == "cpu" {
			if len(f) < 9 {
				return cpu, 0, fmt.Errorf("short cpu line")
			}
			v, e := uints(f[1:9])
			if e != nil {
				return cpu, 0, e
			} // proc order has idle before iowait.
			cpu = [8]uint64{v[0], v[1], v[2], v[4], v[5], v[6], v[7], v[3]}
			seen = true
		} else if strings.HasPrefix(f[0], "cpu") {
			if _, e := strconv.Atoi(f[0][3:]); e == nil {
				cpus++
			}
		}
	}
	if !seen || cpus == 0 {
		return cpu, cpus, fmt.Errorf("missing cpu counters")
	}
	return cpu, cpus, nil
}
func ParseLoad(text string) ([3]float64, error) {
	var out [3]float64
	f := strings.Fields(text)
	if len(f) < 3 {
		return out, fmt.Errorf("short loadavg")
	}
	for i := range out {
		v, e := strconv.ParseFloat(f[i], 64)
		if e != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			return out, fmt.Errorf("invalid loadavg")
		}
		out[i] = v
	}
	return out, nil
}
func ParseUptime(text string) (float64, error) {
	f := strings.Fields(text)
	if len(f) < 1 {
		return 0, fmt.Errorf("missing uptime")
	}
	v, e := strconv.ParseFloat(f[0], 64)
	if e != nil || v < 0 || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, fmt.Errorf("invalid uptime")
	}
	return v, nil
}
func ParseMem(text string) (map[string]uint64, error) {
	m := map[string]uint64{}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) < 2 {
			return nil, fmt.Errorf("invalid meminfo")
		}
		v, e := strconv.ParseUint(f[1], 10, 64)
		if e != nil {
			return nil, e
		}
		if len(f) > 2 {
			if f[2] != "kB" || v > math.MaxUint64/1024 {
				return nil, fmt.Errorf("invalid memory unit")
			}
			v *= 1024
		}
		m[strings.TrimSuffix(f[0], ":")] = v
	}
	return m, nil
}
func ParseNet(text string) (map[string][]uint64, error) {
	m := map[string][]uint64{}
	for _, line := range strings.Split(text, "\n") {
		a, b, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		f := strings.Fields(b)
		if len(f) < 16 {
			return nil, fmt.Errorf("short net/dev")
		}
		v, e := uints(f)
		if e != nil {
			return nil, e
		}
		m[strings.TrimSpace(a)] = []uint64{v[0], v[8], v[2], v[3], v[10], v[11]}
	}
	if len(m) == 0 {
		return nil, fmt.Errorf("missing net devices")
	}
	return m, nil
}
func ParseDiskStats(text string) (map[string][]uint64, error) {
	m := map[string][]uint64{}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if len(f) < 14 {
			return nil, fmt.Errorf("short diskstats")
		}
		v, e := uints(f[3:14])
		if e != nil {
			return nil, e
		}
		m[f[2]] = []uint64{v[2], v[6], v[0], v[4], v[9], v[3], v[7]}
	}
	return m, nil
}

type Mount struct {
	Device, Point, Root, FSType string
	Major                       int
}

func unescape(s string) (string, error) {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		if i+3 >= len(s) {
			return "", fmt.Errorf("short mount escape")
		}
		n, e := strconv.ParseUint(s[i+1:i+4], 8, 8)
		if e != nil {
			return "", e
		}
		b.WriteByte(byte(n))
		i += 3
	}
	return b.String(), nil
}
func ParseMounts(text string) ([]Mount, error) {
	out := []Mount{}
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		a, b, ok := strings.Cut(line, " - ")
		f := strings.Fields(a)
		g := strings.Fields(b)
		if !ok || len(f) < 6 || len(g) < 3 {
			return nil, fmt.Errorf("invalid mountinfo")
		}
		maj, _, ok := strings.Cut(f[2], ":")
		n, e := strconv.Atoi(maj)
		if !ok || e != nil {
			return nil, fmt.Errorf("invalid mount device")
		}
		point, e := unescape(f[4])
		if e != nil {
			return nil, e
		}
		root, e := unescape(f[3])
		if e != nil {
			return nil, e
		}
		out = append(out, Mount{f[2], point, root, g[0], n})
	}
	return out, nil
}

// SelectMounts never invokes statfs on network, virtual or bind filesystems.
func SelectMounts(mounts []Mount) []Mount {
	m := map[string]Mount{}
	for _, v := range mounts {
		allowed := v.FSType == "ext2" || v.FSType == "ext3" || v.FSType == "ext4" || v.FSType == "xfs" || v.FSType == "vfat"
		if !allowed || v.Major <= 0 || v.Root != "/" {
			continue
		}
		old, ok := m[v.Device]
		if !ok || len(v.Point) < len(old.Point) || len(v.Point) == len(old.Point) && v.Point < old.Point {
			m[v.Device] = v
		}
	}
	out := []Mount{}
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

type Process struct {
	Comm  string
	State byte
	RSS   uint64
	Start uint64
}

func ParsePIDStat(text string, pageSize uint64) (Process, error) {
	var p Process
	start := strings.IndexByte(text, '(')
	end := strings.LastIndexByte(text, ')')
	if start < 0 || end <= start {
		return p, fmt.Errorf("invalid pid stat comm")
	}
	f := strings.Fields(text[end+1:])
	if len(f) < 22 || len(f[0]) != 1 {
		return p, fmt.Errorf("short pid stat")
	}
	rss, e := strconv.ParseInt(f[21], 10, 64)
	if e != nil || rss < 0 || pageSize == 0 || uint64(rss) > math.MaxUint64/pageSize {
		return p, fmt.Errorf("invalid rss")
	}
	starttime, e := strconv.ParseUint(f[19], 10, 64)
	if e != nil {
		return p, fmt.Errorf("invalid starttime: %w", e)
	}
	return Process{Comm: text[start+1 : end], State: f[0][0], RSS: uint64(rss) * pageSize, Start: starttime}, nil
}

var TCPColumns = []string{"established", "syn_sent", "syn_recv", "fin_wait1", "fin_wait2", "time_wait", "close", "close_wait", "last_ack", "listen", "closing", "other"}

func ParseTCP(text string) ([12]uint64, error) {
	var out [12]uint64
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] == "sl" {
			continue
		}
		if len(f) < 4 {
			return out, fmt.Errorf("short tcp row")
		}
		state, e := strconv.ParseUint(f[3], 16, 8)
		if e != nil {
			return out, e
		}
		if state >= 1 && state <= 11 {
			out[state-1]++
		} else {
			out[11]++
		}
	}
	return out, nil
}
