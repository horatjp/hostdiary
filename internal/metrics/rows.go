package metrics

import (
	"sort"

	"hostdiary/internal/config"
	"hostdiary/internal/procfs"
	"hostdiary/internal/store"
)

type Observation struct {
	BootID      string              `json:"boot_id"`
	Uptime      float64             `json:"uptime"`
	UptimeValid bool                `json:"uptime_valid"`
	CPU         [8]uint64           `json:"cpu"`
	CPUValid    bool                `json:"cpu_valid"`
	Net         map[string][]uint64 `json:"net"`
	DiskIO      map[string][]uint64 `json:"diskio"`
	PSI         map[string]uint64   `json:"psi"`
	VMStat      map[string]uint64   `json:"vmstat"`
	TCPSegs     []uint64            `json:"tcp_segs"`
}

func Observe(s procfs.Snapshot) Observation {
	return Observation{BootID: s.BootID, Uptime: s.Uptime, UptimeValid: s.UptimeValid,
		CPU: s.CPU, CPUValid: s.CPUValid, Net: s.Net, DiskIO: s.DiskIO,
		PSI: s.PSI, VMStat: s.VMStat, TCPSegs: s.TCPSegs}
}

type Row struct {
	Group  store.Group
	Values []any
}

// BuildRows preserves missing values as nil; valid gauges do not require a baseline.
func BuildRows(prev Observation, s procfs.Snapshot, c config.Config, at int64) ([]Row, []store.Unavailable) {
	cur := Observe(s)
	dt := cur.Uptime - prev.Uptime
	if !prev.UptimeValid || !cur.UptimeValid {
		dt = 0
	}
	out := []Row{}
	missing := append([]store.Unavailable{}, s.Unavailable...)
	// A failed source must retain its error reason in dependent groups/columns.
	sourceReason := func(group string) string {
		for _, u := range s.Unavailable {
			if u.Group == group && u.Reason == "error" {
				return "error"
			}
		}
		return "unsupported"
	}
	add := func(kind, name string, cols, units, desc []string, values []any) {
		cols, units, desc, values = appendExtra(prev, s, c.Apps[name], kind, name, dt, cols, units, desc, values)
		id := store.GroupID(kind, name)
		g := store.Group{ID: id, Kind: kind, Name: name, Columns: append([]string{"time"}, cols...), Units: append([]string{"s"}, units...), Descriptions: append([]string{"collection time (Unix seconds)"}, desc...), LastSeen: at}
		out = append(out, Row{g, append([]any{at}, values...)})
	}
	repeated := func(v string, n int) []string {
		out := make([]string, n)
		for i := range out {
			out[i] = v
		}
		return out
	}
	vals := []any{}
	cpu := CPUPercent(CPUTimes{prev.CPU, prev.BootID, prev.CPUValid}, CPUTimes{cur.CPU, cur.BootID, cur.CPUValid}, dt)
	for _, k := range CPUColumns {
		vals = append(vals, Rounded(cpu[k]))
	}
	add("cpu", "cpu", CPUColumns, repeated("%", 8), cpuDescriptions, vals)
	vals = make([]any, 4)
	if s.LoadValid {
		for i, v := range s.Load {
			vals[i] = Rounded(&v)
		}
	}
	if s.CPUValid {
		vals[3] = s.CPUs
	}
	add("load", "load", []string{"load1", "load5", "load15", "cpus"}, []string{"(none)", "(none)", "(none)", "count"}, []string{"1 minute load", "5 minute load", "15 minute load", "logical CPU count"}, vals)
	mem := func(key, kind, col string) any {
		if v, ok := s.Mem[key]; ok {
			return v
		}
		column := col
		missing = append(missing, store.Unavailable{Group: kind, Column: &column, Reason: sourceReason("mem")})
		return nil
	}
	total := mem("MemTotal", "mem", "total")
	available := mem("MemAvailable", "mem", "available")
	var used any
	if total != nil && available != nil && total.(uint64) >= available.(uint64) {
		used = total.(uint64) - available.(uint64)
	}
	add("mem", "mem", []string{"total", "available", "used", "buffers", "cached"}, repeated("bytes", 5), []string{"physical memory", "available memory", "total minus available", "block buffers", "page cache; includes shmem and tmpfs"}, []any{total, available, used, mem("Buffers", "mem", "buffers"), mem("Cached", "mem", "cached")})
	st := mem("SwapTotal", "swap", "total")
	sf := mem("SwapFree", "swap", "used")
	var su any
	if st != nil && sf != nil && st.(uint64) >= sf.(uint64) {
		su = st.(uint64) - sf.(uint64)
	}
	add("swap", "swap", []string{"total", "used"}, repeated("bytes", 2), []string{"swap capacity", "swap used"}, []any{st, su})
	vals = make([]any, 7)
	if s.ProcessesValid {
		counts := [7]uint64{}
		for _, p := range s.Processes {
			counts[0]++
			i := 6
			switch p.State {
			case 'R':
				i = 1
			case 'S':
				i = 2
			case 'D':
				i = 3
			case 'T', 't':
				i = 4
			case 'Z':
				i = 5
			}
			counts[i]++
		}
		for i, v := range counts {
			vals[i] = v
		}
	}
	add("procs", "procs", []string{"total", "running", "sleeping", "disk_sleep", "stopped", "zombie", "other"}, repeated("count", 7), []string{"all visible processes", "running", "sleeping", "uninterruptible sleep", "stopped or traced", "zombie", "other; includes kernel idle threads"}, vals)
	vals = make([]any, 12)
	if s.TCPValid {
		for i, v := range s.TCP {
			vals[i] = v
		}
	}
	add("tcp", "tcp", procfs.TCPColumns, repeated("count", 12), tcpDescriptions, vals)
	for name, v := range s.Net {
		vals = make([]any, 2)
		p := prev.Net[name]
		for i, n := range v[:min(len(v), 2)] {
			var pc Counter
			if i < len(p) {
				pc = Counter{p[i], prev.BootID, true}
			}
			vals[i] = Rounded(Rate(pc, Counter{n, cur.BootID, true}, dt))
		}
		add("net", name, []string{"rx", "tx"}, repeated("bytes/s", 2), []string{"received bytes per second", "transmitted bytes per second"}, vals)
	}
	for name, v := range s.DiskIO {
		vals = make([]any, 5)
		p := prev.DiskIO[name]
		for i, n := range v[:min(len(v), 5)] {
			var pc Counter
			if i < len(p) {
				pc = Counter{p[i], prev.BootID, true}
			}
			r := Rate(pc, Counter{n, cur.BootID, true}, dt)
			if r != nil {
				if i < 2 {
					*r *= 512
				}
				if i == 4 {
					*r /= 10
				}
			}
			vals[i] = Rounded(r)
		}
		add("diskio", name, []string{"read", "write", "read_ops", "write_ops", "busy"}, []string{"bytes/s", "bytes/s", "count/s", "count/s", "%"}, []string{"sectors read times 512 bytes", "sectors written times 512 bytes", "completed reads per second", "completed writes per second", "approximate busy percentage; may exceed 100 slightly"}, vals)
	}
	for name, v := range s.Disks {
		var iu, it any
		if v.InodesTotal > 0 {
			iu = v.InodesUsed
			it = v.InodesTotal
		} else {
			for _, col := range []string{"inodes_used", "inodes_total"} {
				column := col
				missing = append(missing, store.Unavailable{Group: store.GroupID("disk", name), Column: &column, Reason: "unsupported"})
			}
		}
		add("disk", name, []string{"used", "avail", "inodes_used", "inodes_total"}, []string{"bytes", "bytes", "count", "count"}, []string{"allocated blocks times fragment size", "available to unprivileged users", "allocated inodes", "total inodes"}, []any{v.Used, v.Avail, iu, it})
	}
	for name, aliases := range c.Apps {
		vals = make([]any, 2)
		if s.ProcessesValid {
			var count, rss uint64
			for _, p := range s.Processes {
				if p.State == 'Z' {
					continue
				}
				for _, alias := range aliases {
					if p.Comm == alias {
						count++
						rss += p.RSS
						break
					}
				}
			}
			vals = []any{count, rss}
		} else {
			missing = append(missing, store.Unavailable{Group: store.GroupID("app", name), Reason: sourceReason("procs")})
		}
		add("app", name, []string{"count", "rss"}, []string{"count", "bytes"}, []string{"matching non-zombie processes; rename app when matching rules change", "sum of RSS; shared memory counted multiple times"}, vals)
	}
	add("psi", "psi", nil, nil, nil, nil)
	sort.Slice(out, func(i, j int) bool { return out[i].Group.ID < out[j].Group.ID })
	return out, normalizeUnavailable(missing)
}
