package store

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"
)

// day0 is a UTC midnight used by the pure summary tests.
const day0 = int64(1794096000)

func f(v float64) *float64 { return &v }
func row(at int64, values map[string]*float64) namedRow {
	return namedRow{Time: at, Values: values}
}
func memHeader() Header {
	return Header{1, "mem", "mem", "mem", 60, []string{"time", "available", "oom_kills"}, []string{"s", "bytes", "count"}}
}
func column(g GroupDay, name string) ColumnSummary {
	for _, c := range g.Columns {
		if c.Name == name {
			return c
		}
	}
	return ColumnSummary{}
}

func TestSummarizeGroupDayRejectsInvalidInput(t *testing.T) {
	if _, e := summarizeGroupDay(memHeader(), nil, day0+1, day0); e == nil {
		t.Fatal("day start must be a UTC midnight")
	}
	h := memHeader()
	h.Units = h.Units[:2]
	if _, e := summarizeGroupDay(h, nil, day0, day0); e == nil {
		t.Fatal("columns and units must have the same length")
	}
	h = memHeader()
	h.Columns[0] = "at"
	if _, e := summarizeGroupDay(h, nil, day0, day0); e == nil {
		t.Fatal("first column must be time")
	}
}

func TestSummarizeGroupDayStatistics(t *testing.T) {
	rows := []namedRow{
		row(day0, map[string]*float64{"available": f(500), "oom_kills": f(0)}),
		row(day0+60, map[string]*float64{"available": f(300), "oom_kills": f(2)}),
		row(day0+120, map[string]*float64{"available": f(300), "oom_kills": nil}),
		row(day0+180, map[string]*float64{"available": f(500), "oom_kills": f(1)}),
	}
	g, e := summarizeGroupDay(memHeader(), rows, day0, day0+86400*2)
	if e != nil {
		t.Fatal(e)
	}
	if g.ID != "mem" || g.Kind != "mem" || g.Name != "mem" || g.Rows != 4 || g.First != day0 || g.Last != day0+180 {
		t.Fatalf("group fields: %+v", g)
	}
	a := column(g, "available")
	// Ties keep the earliest time for both the minimum and the maximum.
	if a.Unit != "bytes" || a.Count != 4 || *a.Min != 300 || *a.MinTime != day0+60 || *a.Max != 500 || *a.MaxTime != day0 || *a.Mean != 400 || a.Sum != nil {
		t.Fatalf("available: %+v", a)
	}
	o := column(g, "oom_kills")
	if o.Count != 3 || *o.Sum != 3 || *o.Max != 2 || *o.MaxTime != day0+60 {
		t.Fatalf("oom_kills: %+v", o)
	}
	if !reflect.DeepEqual([]string{g.Columns[0].Name, g.Columns[1].Name}, []string{"available", "oom_kills"}) {
		t.Fatalf("column order follows the header: %+v", g.Columns)
	}
}

func TestSummarizeGroupDayAllNullAndEmpty(t *testing.T) {
	g, e := summarizeGroupDay(memHeader(), []namedRow{row(day0, map[string]*float64{"available": nil})}, day0, day0+86400)
	if e != nil {
		t.Fatal(e)
	}
	o := column(g, "oom_kills")
	if o.Count != 0 || o.Min != nil || o.MinTime != nil || o.Max != nil || o.MaxTime != nil || o.Mean != nil {
		t.Fatalf("all-null column: %+v", o)
	}
	if o.Sum != nil {
		t.Fatal("sum stays null without values")
	}
	g, e = summarizeGroupDay(memHeader(), nil, day0, day0+86400)
	if e != nil {
		t.Fatal(e)
	}
	if g.Rows != 0 || g.First != 0 || g.Last != 0 {
		t.Fatalf("empty input: %+v", g)
	}
}

func TestSummarizeGroupDayDuplicatesRangeAndHeaderChange(t *testing.T) {
	rows := []namedRow{
		row(day0-60, map[string]*float64{"available": f(1)}),                    // previous day
		row(day0, map[string]*float64{"available": f(9)}),                       // replaced below
		row(day0, map[string]*float64{"available": f(7)}),                       // later row wins
		row(day0+86400, map[string]*float64{"available": f(1)}),                 // next day
		row(day0+60, map[string]*float64{"available": f(8), "oom_kills": f(0)}), // column added later
	}
	g, e := summarizeGroupDay(memHeader(), rows, day0, day0+86400)
	if e != nil {
		t.Fatal(e)
	}
	a := column(g, "available")
	if g.Rows != 2 || a.Count != 2 || *a.Min != 7 || *a.Max != 8 {
		t.Fatalf("rows=%d available=%+v", g.Rows, a)
	}
	if o := column(g, "oom_kills"); o.Count != 1 {
		t.Fatalf("absent values in older rows are null: %+v", o)
	}
}

func TestSummarizeGroupDayMeanRounding(t *testing.T) {
	h := Header{1, "cpu", "cpu", "cpu", 60, []string{"time", "user"}, []string{"s", "%"}}
	g, _ := summarizeGroupDay(h, []namedRow{row(day0, map[string]*float64{"user": f(0.1)}), row(day0+60, map[string]*float64{"user": f(0.2)}), row(day0+120, map[string]*float64{"user": f(0.2)})}, day0, day0+86400)
	if m := *column(g, "user").Mean; m != 0.167 {
		t.Fatalf("mean rounds to 3 decimals: %v", m)
	}
	g, _ = summarizeGroupDay(h, []namedRow{row(day0, map[string]*float64{"user": f(-0.0001)})}, day0, day0+86400)
	b, _ := json.Marshal(column(g, "user").Mean)
	if string(b) != "0" || math.Signbit(*column(g, "user").Mean) {
		t.Fatalf("negative zero is normalized: %s", b)
	}
}

func TestSummarizeGroupDaySumOnlyForEventCounts(t *testing.T) {
	app := Header{1, "app-web-1", "app", "web", 60, []string{"time", "count", "rss", "started"}, []string{"s", "count", "bytes", "count"}}
	g, _ := summarizeGroupDay(app, []namedRow{row(day0, map[string]*float64{"count": f(3), "rss": f(10), "started": f(1)})}, day0, day0+86400)
	if column(g, "count").Sum != nil || column(g, "rss").Sum != nil || *column(g, "started").Sum != 1 {
		t.Fatalf("sum: %+v", g.Columns)
	}
	procs := Header{1, "procs", "procs", "procs", 60, []string{"time", "started"}, []string{"s", "count"}}
	g, _ = summarizeGroupDay(procs, []namedRow{row(day0, map[string]*float64{"started": f(1)})}, day0, day0+86400)
	if column(g, "started").Sum != nil {
		t.Fatal("sum is limited to mem.oom_kills and app.started")
	}
}

func minutes(n ...int) []namedRow {
	out := []namedRow{}
	for _, m := range n {
		out = append(out, row(day0+int64(m*60), map[string]*float64{"available": f(1)}))
	}
	return out
}

func TestSummarizeGroupDayMissingMinutes(t *testing.T) {
	full := []int{}
	for i := 0; i < 1440; i++ {
		full = append(full, i)
	}
	tests := []struct {
		name     string
		rows     []namedRow
		at       int64
		missing  int
		gaps     []Gap
		expected int
	}{
		{"complete day", minutes(full...), day0 + 86400, 0, []Gap{}, 1440},
		{"partial at midnight", minutes(0), day0, 0, []Gap{}, 1},
		{"partial in progress", minutes(0, 1, 3), day0 + 3*60 + 30, 1, []Gap{{day0 + 120, day0 + 120, 1}}, 4},
		{"leading and trailing", minutes(2, 3), day0 + 86400, 1438, []Gap{{day0, day0 + 60, 2}, {day0 + 240, day0 + 1439*60, 1436}}, 1440},
		{"two rows in one minute slot", append(minutes(full...), row(day0+600+32, map[string]*float64{"available": f(1)})), day0 + 86400, 0, []Gap{}, 1440},
		{"rows after the expected end", minutes(0, 5), day0 + 60, 1, []Gap{{day0 + 60, day0 + 60, 1}}, 2},
	}
	for _, tt := range tests {
		g, e := summarizeGroupDay(memHeader(), tt.rows, day0, tt.at)
		if e != nil {
			t.Fatal(tt.name, e)
		}
		if g.MissingMinutes == nil || *g.MissingMinutes != tt.missing || !reflect.DeepEqual(g.Gaps, tt.gaps) || g.GapsTruncated {
			t.Fatalf("%s: missing=%v gaps=%+v truncated=%v", tt.name, g.MissingMinutes, g.Gaps, g.GapsTruncated)
		}
		if _, expected := dayStatus(day0, tt.at); expected == nil || *expected != tt.expected {
			t.Fatalf("%s: expected minutes %v", tt.name, expected)
		}
	}
	if g, _ := summarizeGroupDay(memHeader(), minutes(0), day0, day0+86400*3); g.Rows != 1 {
		t.Fatal("expected minutes stop at 1440")
	}
}

func TestSummarizeGroupDayGapLimit(t *testing.T) {
	// Rows on even minutes only: gap k covers minute 2k+1.
	for _, n := range []int{20, 21} {
		ms := []int{}
		for i := 0; i <= n; i++ {
			ms = append(ms, 2*i)
		}
		g, _ := summarizeGroupDay(memHeader(), minutes(ms...), day0, day0+int64(2*n*60))
		if len(g.Gaps) != min(n, 20) || g.GapsTruncated != (n > 20) || *g.MissingMinutes != n {
			t.Fatalf("n=%d gaps=%d truncated=%v missing=%d", n, len(g.Gaps), g.GapsTruncated, *g.MissingMinutes)
		}
	}
}

func TestDayStatusAndFutureDay(t *testing.T) {
	for _, tt := range []struct {
		at       int64
		status   string
		expected *int
	}{
		{day0 + 86400, "complete", func() *int { v := 1440; return &v }()},
		{day0 + 86399, "partial", func() *int { v := 1440; return &v }()},
		{day0 - 1, "future", nil},
	} {
		s, e := dayStatus(day0, tt.at)
		if s != tt.status || !reflect.DeepEqual(e, tt.expected) {
			t.Fatalf("at=%d: %s %v", tt.at, s, e)
		}
	}
	g, _ := summarizeGroupDay(memHeader(), minutes(0), day0, day0-1)
	if g.MissingMinutes != nil || len(g.Gaps) != 0 || g.Gaps == nil || g.Rows != 1 {
		t.Fatalf("future day: %+v", g)
	}
}
