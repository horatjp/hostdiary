// summary.go — per-day statistics of retained minute values for AI readers.
//
// What: summary.json lists, for each retained UTC day and group, row counts,
// missing minute slots and per-column min/max/mean (sum for event counts).
// Why: readers should not re-implement these scans, and the numbers keep the
// original units so per-second rates are never silently rescaled
// (change ai-summary-and-guide). No thresholds or judgements are produced.
package store

import (
	"fmt"
	"math"
	"sort"
)

// maxGaps bounds the gap list so a sparse day cannot bloat summary.json.
const maxGaps = 20

type Gap struct {
	From    int64 `json:"from"`
	To      int64 `json:"to"`
	Minutes int   `json:"minutes"`
}
type ColumnSummary struct {
	Name    string   `json:"name"`
	Unit    string   `json:"unit"`
	Count   int      `json:"count"`
	Min     *float64 `json:"min"`
	MinTime *int64   `json:"min_time"`
	Max     *float64 `json:"max"`
	MaxTime *int64   `json:"max_time"`
	Mean    *float64 `json:"mean"`
	Sum     *float64 `json:"sum,omitempty"`
}
type GroupDay struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Name           string          `json:"name"`
	Rows           int             `json:"rows"`
	First          int64           `json:"first"`
	Last           int64           `json:"last"`
	MissingMinutes *int            `json:"missing_minutes"`
	Gaps           []Gap           `json:"gaps"`
	GapsTruncated  bool            `json:"gaps_truncated"`
	Columns        []ColumnSummary `json:"columns"`
}

// eventCount reports columns whose per-minute values are event counts, so a
// daily total is meaningful. Other "count" units are states (processes,
// connections, inodes) and must not be summed.
func eventCount(kind, column string) bool {
	return kind == "mem" && column == "oom_kills" || kind == "app" && column == "started"
}

// dayStatus classifies a UTC day against the collection time at and returns
// the number of minute slots expected so far (nil for a future day).
func dayStatus(dayStart, at int64) (string, *int) {
	switch {
	case at >= dayStart+86400:
		n := 1440
		return "complete", &n
	case at >= dayStart:
		n := min(1440, int((at-dayStart)/60)+1)
		return "partial", &n
	}
	return "future", nil
}

// summarizeGroupDay is pure: rows outside the day are ignored, a later row with
// the same time replaces an earlier one (as in rollup), and columns follow h.
func summarizeGroupDay(h Header, rows []namedRow, dayStart, at int64) (GroupDay, error) {
	if dayStart%86400 != 0 {
		return GroupDay{}, fmt.Errorf("summary: day start %d is not a UTC midnight", dayStart)
	}
	if len(h.Columns) == 0 || h.Columns[0] != "time" || len(h.Columns) != len(h.Units) {
		return GroupDay{}, fmt.Errorf("summary: invalid columns or units for %s", h.Group)
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
	g := GroupDay{ID: h.Group, Kind: h.Kind, Name: h.Name, Rows: len(times), Gaps: []Gap{}, Columns: []ColumnSummary{}}
	if len(times) > 0 {
		g.First, g.Last = times[0], times[len(times)-1]
	}
	if _, expected := dayStatus(dayStart, at); expected != nil {
		g.MissingMinutes, g.Gaps, g.GapsTruncated = missingSlots(times, dayStart, *expected)
	}
	for i, name := range h.Columns[1:] {
		c := ColumnSummary{Name: name, Unit: h.Units[i+1]}
		var total float64
		for _, t := range times {
			v := unique[t].Values[name]
			if v == nil {
				continue
			}
			at := t
			if c.Count == 0 || *v < *c.Min {
				c.Min, c.MinTime = f64(*v), &at
			}
			if c.Count == 0 || *v > *c.Max {
				c.Max, c.MaxTime = f64(*v), &at
			}
			total += *v
			c.Count++
		}
		if c.Count > 0 {
			c.Mean = f64(round3(total / float64(c.Count)))
			if eventCount(h.Kind, name) {
				c.Sum = f64(total)
			}
		}
		g.Columns = append(g.Columns, c)
	}
	return g, nil
}

// missingSlots counts minute slots 0..expected-1 without a row and lists the
// first maxGaps runs. times must be sorted and within the day.
func missingSlots(times []int64, dayStart int64, expected int) (*int, []Gap, bool) {
	present := make([]bool, expected)
	for _, t := range times {
		if s := int((t - dayStart) / 60); s < expected {
			present[s] = true
		}
	}
	missing := 0
	gaps := []Gap{}
	truncated := false
	for s := 0; s < expected; s++ {
		if present[s] {
			continue
		}
		missing++
		if s > 0 && !present[s-1] {
			if !truncated {
				gaps[len(gaps)-1].To = dayStart + int64(s)*60
				gaps[len(gaps)-1].Minutes++
			}
			continue
		}
		if len(gaps) == maxGaps {
			truncated = true
			continue
		}
		gaps = append(gaps, Gap{dayStart + int64(s)*60, dayStart + int64(s)*60, 1})
	}
	return &missing, gaps, truncated
}

func f64(v float64) *float64 { return &v }

func round3(v float64) float64 {
	r := math.Round(v*1000) / 1000
	if r == 0 {
		return 0 // Normalize IEEE negative zero before JSON serialization.
	}
	return r
}
