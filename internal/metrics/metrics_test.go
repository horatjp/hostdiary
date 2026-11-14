package metrics

import (
	"math"
	"testing"
)

// Rate accepts only a same-boot, monotonic counter over (0,180] seconds.
func TestRateContract(t *testing.T) {
	for _, tc := range []struct {
		name      string
		prev, cur Counter
		dt        float64
		want      *float64
	}{
		{"normal", Counter{100, "a", true}, Counter{220, "a", true}, 60, ptr(2)},
		{"boundary", Counter{100, "a", true}, Counter{280, "a", true}, 180, ptr(1)},
		{"missing", Counter{}, Counter{220, "a", true}, 60, nil},
		{"reboot", Counter{100, "a", true}, Counter{220, "b", true}, 60, nil},
		{"decrease", Counter{100, "a", true}, Counter{99, "a", true}, 60, nil},
		{"zero", Counter{100, "a", true}, Counter{220, "a", true}, 0, nil},
		{"negative", Counter{100, "a", true}, Counter{220, "a", true}, -1, nil},
		{"gap", Counter{100, "a", true}, Counter{220, "a", true}, 181, nil},
		{"nan", Counter{100, "a", true}, Counter{220, "a", true}, math.NaN(), nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := Rate(tc.prev, tc.cur, tc.dt)
			if (got == nil) != (tc.want == nil) || got != nil && *got != *tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

// CPU percentages clamp negative column deltas, then sum to 100 or are all nil.
func TestCPUContract(t *testing.T) {
	prev := CPUTimes{Values: [8]uint64{10, 10, 10, 10, 10, 10, 10, 10}, BootID: "a", Valid: true}
	cur := prev
	cur.Values = [8]uint64{20, 20, 20, 9, 20, 20, 20, 20}
	for _, dt := range []float64{0, -1, 181, 60, 180} {
		got := CPUPercent(prev, cur, dt)
		sum := 0.0
		for _, v := range got {
			if dt <= 0 || dt > 180 {
				if v != nil {
					t.Fatal("invalid interval")
				}
			} else {
				if v == nil || *v < 0 || *v > 100 {
					t.Fatal(got)
				}
				sum += *v
			}
		}
		if dt > 0 && dt <= 180 && math.Abs(sum-100) > .01 {
			t.Fatal(sum)
		}
	}
	for _, p := range []CPUTimes{{}, cur, {Values: prev.Values, BootID: "b", Valid: true}} {
		for _, v := range CPUPercent(p, cur, 60) {
			if v != nil {
				t.Fatal("expected nil")
			}
		}
	}
}
