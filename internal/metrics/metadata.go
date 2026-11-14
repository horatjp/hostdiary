package metrics

import (
	"sort"

	"hostdiary/internal/store"
)

const cpuInterval = "percentage of total time across all logical CPUs since previous collection (interval average); columns sum to 100; "

var cpuDescriptions = []string{
	cpuInterval + "user time, including guest time",
	cpuInterval + "user time for niced processes",
	cpuInterval + "kernel execution time",
	cpuInterval + "I/O wait; negative counter differences are clamped to zero",
	cpuInterval + "hardware interrupt handling time",
	cpuInterval + "software interrupt handling time",
	cpuInterval + "steal time taken by the hypervisor to run other guests",
	cpuInterval + "idle time",
}
var tcpDescriptions = []string{
	"number of TCP sockets in ESTABLISHED state (IPv4 + IPv6)",
	"number of TCP sockets in SYN_SENT state (IPv4 + IPv6)",
	"number of TCP sockets in SYN_RECV state (IPv4 + IPv6)",
	"number of TCP sockets in FIN_WAIT1 state (IPv4 + IPv6)",
	"number of TCP sockets in FIN_WAIT2 state (IPv4 + IPv6)",
	"number of TCP sockets in TIME_WAIT state (IPv4 + IPv6)",
	"number of TCP sockets in CLOSE state (IPv4 + IPv6)",
	"number of TCP sockets in CLOSE_WAIT state (IPv4 + IPv6)",
	"number of TCP sockets in LAST_ACK state (IPv4 + IPv6)",
	"number of TCP sockets in LISTEN state (IPv4 + IPv6)",
	"number of TCP sockets in CLOSING state (IPv4 + IPv6)",
	"number of TCP sockets with state outside 1-11 (IPv4 + IPv6)",
}

// Group-level failures supersede column details even when their reasons differ.
// Return a fresh, deterministic list without modifying the caller's slice.
func normalizeUnavailable(input []store.Unavailable) []store.Unavailable {
	whole := map[string]bool{}
	for _, u := range input {
		if u.Column == nil {
			whole[u.Group] = true
		}
	}
	type key struct {
		group, column, reason string
		whole                 bool
	}
	seen := map[key]bool{}
	out := []store.Unavailable{}
	for _, u := range input {
		if u.Column != nil && whole[u.Group] {
			continue
		}
		k := key{group: u.Group, reason: u.Reason, whole: u.Column == nil}
		if u.Column != nil {
			k.column = *u.Column
		}
		if !seen[k] {
			seen[k] = true
			out = append(out, u)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Group != b.Group {
			return a.Group < b.Group
		}
		if (a.Column == nil) != (b.Column == nil) {
			return a.Column == nil
		}
		if a.Column != nil && *a.Column != *b.Column {
			return *a.Column < *b.Column
		}
		return a.Reason < b.Reason
	})
	return out
}
