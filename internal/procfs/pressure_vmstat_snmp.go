package procfs

import (
	"fmt"
	"strconv"
	"strings"
)

// ParsePSI extracts cumulative microseconds; full is optional, some is required.
func ParsePSI(text string) (some, full *uint64, err error) {
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != "some" && f[0] != "full" {
			continue
		}
		var total *uint64
		for _, field := range f[1:] {
			if value, ok := strings.CutPrefix(field, "total="); ok {
				n, e := strconv.ParseUint(value, 10, 64)
				if e != nil {
					return nil, nil, fmt.Errorf("invalid PSI total: %w", e)
				}
				total = &n
			}
		}
		if total == nil {
			return nil, nil, fmt.Errorf("missing PSI total")
		}
		if f[0] == "some" {
			some = total
		} else {
			full = total
		}
	}
	if some == nil {
		return nil, nil, fmt.Errorf("missing PSI some")
	}
	return some, full, nil
}

// ParseVMStat returns only readable requested counters, retaining measured zero.
func ParseVMStat(text string) map[string]uint64 {
	v, _ := parseVMStat(text)
	return v
}

// Keep malformed counters separate from absent keys for per-column diagnostics.
func parseVMStat(text string) (map[string]uint64, map[string]bool) {
	v, bad := map[string]uint64{}, map[string]bool{}
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != "pswpin" && f[0] != "pswpout" && f[0] != "oom_kill" {
			continue
		}
		if len(f) != 2 {
			bad[f[0]] = true
			continue
		}
		n, e := strconv.ParseUint(f[1], 10, 64)
		if e != nil {
			bad[f[0]] = true
			continue
		}
		v[f[0]] = n
	}
	for key := range bad {
		delete(v, key)
	}
	return v, bad
}

// ParseSNMPTCP matches named columns and ignores signed or unrelated fields.
func ParseSNMPTCP(text string) ([]uint64, error) {
	var header []string
	for _, line := range strings.Split(text, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != "Tcp:" {
			continue
		}
		if header == nil {
			header = f
			continue
		}
		if len(header) != len(f) {
			return nil, fmt.Errorf("TCP SNMP column count mismatch")
		}
		v := make([]uint64, 2)
		for i, name := range []string{"OutSegs", "RetransSegs"} {
			found := false
			for j, col := range header {
				if col == name {
					n, e := strconv.ParseUint(f[j], 10, 64)
					if e != nil {
						return nil, fmt.Errorf("invalid TCP SNMP %s: %w", name, e)
					}
					v[i], found = n, true
				}
			}
			if !found {
				return nil, fmt.Errorf("missing TCP SNMP %s", name)
			}
		}
		return v, nil
	}
	return nil, fmt.Errorf("missing TCP SNMP rows")
}
