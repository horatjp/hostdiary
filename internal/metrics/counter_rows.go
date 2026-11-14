package metrics

import (
	"math"
	"os"

	"hostdiary/internal/procfs"
)

// delta counts events across any interval, independently of rate timing validity.
func delta(prev, cur Counter) *uint64 {
	if !prev.Valid || !cur.Valid || prev.BootID == "" || prev.BootID != cur.BootID || cur.Value < prev.Value {
		return nil
	}
	v := cur.Value - prev.Value
	return &v
}

// started uses USER_HZ=100 ticks; aliases are OR matches, never duplicate counts.
func started(procs []procfs.Process, aliases []string, prevCS, curCS uint64, sameBoot bool) *uint64 {
	if !sameBoot || prevCS >= curCS {
		return nil
	}
	var n uint64
	for _, p := range procs {
		if p.State == 'Z' || p.Start <= prevCS || p.Start > curCS {
			continue
		}
		for _, alias := range aliases {
			if p.Comm == alias {
				n++
				break
			}
		}
	}
	return &n
}

func centiseconds(uptime float64, valid bool) (uint64, bool) {
	v := math.Round(uptime * 100)
	if !valid || uptime < 0 || math.IsNaN(v) || math.IsInf(v, 0) || v >= math.Exp2(64) {
		return 0, false
	}
	return uint64(v), true
}

func countValue(v *uint64) any {
	if v == nil {
		return nil
	}
	return *v
}

// appendExtra keeps existing column order intact and adds only schema extensions.
// Map membership and slice lengths distinguish absent state from measured zero.
func appendExtra(prev Observation, s procfs.Snapshot, aliases []string, kind, name string, dt float64, cols, units, desc []string, values []any) ([]string, []string, []string, []any) {
	add := func(col, unit, description string, value any) {
		cols = append(cols, col)
		units = append(units, unit)
		desc = append(desc, description)
		values = append(values, value)
	}
	mapCounters := func(p, c map[string]uint64, key string) (Counter, Counter) {
		pv, pok := p[key]
		cv, cok := c[key]
		return Counter{pv, prev.BootID, pok}, Counter{cv, s.BootID, cok}
	}
	mapRate := func(p, c map[string]uint64, key string, factor float64) any {
		pc, cc := mapCounters(p, c, key)
		r := Rate(pc, cc, dt)
		if r != nil {
			*r *= factor
		}
		return Rounded(r)
	}
	sliceRate := func(p, c []uint64, index int) any {
		if index >= len(p) || index >= len(c) {
			return nil
		}
		return Rounded(Rate(Counter{p[index], prev.BootID, true}, Counter{c[index], s.BootID, true}, dt))
	}
	switch kind {
	case "psi":
		for _, col := range []string{"cpu_some", "memory_some", "memory_full", "io_some", "io_full"} {
			which := "at least one task (some) was"
			if col == "memory_full" || col == "io_full" {
				which = "all non-idle tasks (full) were"
			}
			add(col, "%", "percentage of time "+which+" stalled on the resource during the interval", mapRate(prev.PSI, s.PSI, col, 1e-4))
		}
	case "mem":
		pc, cc := mapCounters(prev.VMStat, s.VMStat, "oom_kill")
		add("oom_kills", "count", "processes killed by the kernel OOM killer since previous collection; includes cgroup (container) memory-limit kills, so host memory may not have been exhausted; compare with mem.available; no interval upper limit; coarse average times samples approximates the interval total", countValue(delta(pc, cc)))
	case "swap":
		for i, col := range []string{"in", "out"} {
			key := []string{"pswpin", "pswpout"}[i]
			add(col, "bytes/s", "bytes swapped "+col+" per second; pages converted using the runtime page size", mapRate(prev.VMStat, s.VMStat, key, float64(os.Getpagesize())))
		}
	case "tcp":
		for i, col := range []string{"out_segs", "retrans_segs"} {
			var value any
			if s.TCPValid {
				value = sliceRate(prev.TCPSegs, s.TCPSegs, i)
			}
			add(col, "count/s", segmentDescriptions[i], value)
		}
	case "net":
		for i, col := range []string{"rx_errors", "rx_drops", "tx_errors", "tx_drops"} {
			add(col, "count/s", netQualityDescriptions[i], sliceRate(prev.Net[name], s.Net[name], i+2))
		}
	case "diskio":
		for i, col := range []string{"read_time", "write_time"} {
			add(col, "ms/s", diskTimeDescriptions[i], sliceRate(prev.DiskIO[name], s.DiskIO[name], i+5))
		}
	case "app":
		var value any
		p, pok := centiseconds(prev.Uptime, prev.UptimeValid)
		c, cok := centiseconds(s.Uptime, s.UptimeValid)
		if s.ProcessesValid && pok && cok {
			value = countValue(started(s.Processes, aliases, p, c, prev.BootID != "" && prev.BootID == s.BootID))
		}
		add("started", "count", "matching non-zombie processes started since previous collection and still running; processes that started and exited within the interval are not counted; assumes USER_HZ=100; no interval upper limit; coarse average times samples approximates the interval total", value)
	}
	return cols, units, desc, values
}
