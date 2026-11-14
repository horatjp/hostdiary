// Package metrics implements pure monotonic-counter and row calculations.
package metrics

import "math"

type Counter struct {
	Value  uint64
	BootID string
	Valid  bool
}
type CPUTimes struct {
	Values [8]uint64
	BootID string
	Valid  bool
}

var CPUColumns = []string{"user", "nice", "system", "iowait", "irq", "softirq", "steal", "idle"}

func ptr(v float64) *float64  { return &v }
func validDT(dt float64) bool { return dt > 0 && dt <= 180 && !math.IsNaN(dt) }
func Rate(prev, cur Counter, dt float64) *float64 {
	if !prev.Valid || !cur.Valid || prev.BootID == "" || prev.BootID != cur.BootID || cur.Value < prev.Value || !validDT(dt) {
		return nil
	}
	return ptr(float64(cur.Value-prev.Value) / dt)
}
func CPUPercent(prev, cur CPUTimes, dt float64) map[string]*float64 {
	out := map[string]*float64{}
	for _, k := range CPUColumns {
		out[k] = nil
	}
	if !prev.Valid || !cur.Valid || prev.BootID == "" || prev.BootID != cur.BootID || !validDT(dt) {
		return out
	}
	var delta [8]float64
	total := 0.0
	for i, v := range cur.Values {
		if v >= prev.Values[i] {
			delta[i] = float64(v - prev.Values[i])
		}
		total += delta[i]
	}
	if total > 0 {
		for i, k := range CPUColumns {
			out[k] = ptr(delta[i] * 100 / total)
		}
	}
	return out
}

// Rounded returns a JSON-safe value at the public schema's three-digit precision.
func Rounded(v *float64) any {
	if v == nil || math.IsNaN(*v) || math.IsInf(*v, 0) {
		return nil
	}
	return math.Round(*v*1000) / 1000
}
