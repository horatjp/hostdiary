// Coarse values are computed directly from named minute rows, never averages of averages.
package store

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

type namedRow struct {
	Time   int64
	Values map[string]*float64
}
type bucket struct {
	Time      int64
	Samples   int
	LastTime  int64
	Mean, Max map[string]*float64
}

func coarseColumns(columns, units []string) ([]string, []string, error) {
	if len(columns) == 0 || columns[0] != "time" || len(columns) != len(units) {
		return nil, nil, fmt.Errorf("invalid minute columns or units")
	}
	c, u := []string{"time", "samples", "last_time"}, []string{"s", "count", "s"}
	for i, name := range columns[1:] {
		if name == "samples" || name == "last_time" || strings.HasSuffix(name, "_max") {
			return nil, nil, fmt.Errorf("reserved coarse column: %s", name)
		}
		c = append(c, name, name+"_max")
		u = append(u, units[i+1], units[i+1])
	}
	return c, u, nil
}

func aggregate(rows []namedRow, step, dayStart int64) ([]bucket, error) {
	if (step != 300 && step != 3600 && step != 86400) || dayStart%86400 != 0 {
		return nil, fmt.Errorf("invalid rollup step or UTC day start")
	}
	unique := map[int64]namedRow{}
	for _, r := range rows {
		if r.Time >= dayStart && r.Time < dayStart+86400 {
			unique[r.Time] = r
		}
	}
	times := make([]int64, 0, len(unique))
	for t := range unique {
		times = append(times, t)
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	out := []bucket{}
	var counts map[string]int
	for _, t := range times {
		start := dayStart + (t-dayStart)/step*step
		if len(out) == 0 || out[len(out)-1].Time != start {
			out = append(out, bucket{Time: start, Mean: map[string]*float64{}, Max: map[string]*float64{}})
			counts = map[string]int{}
		}
		b := &out[len(out)-1]
		b.Samples++
		b.LastTime = t
		for c, v := range unique[t].Values {
			if v == nil {
				continue
			}
			n := counts[c]
			mean, peak := *v, *v
			if n > 0 {
				mean = *b.Mean[c] + (*v-*b.Mean[c])/float64(n+1)
				peak = math.Max(*b.Max[c], *v)
			}
			b.Mean[c] = &mean
			b.Max[c] = &peak
			counts[c]++
		}
	}
	for i := range out {
		for c, v := range out[i].Mean {
			rounded := math.Round(*v*1000) / 1000
			if rounded == 0 {
				rounded = 0 // Normalize IEEE negative zero before JSON serialization.
			}
			out[i].Mean[c] = &rounded
		}
	}
	return out, nil
}
