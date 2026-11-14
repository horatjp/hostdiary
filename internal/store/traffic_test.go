package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestTrafficEstimateContract(t *testing.T) {
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	tr := trafficEstimate(nil, &partial{RX: ptr(2), TX: ptr(3), Samples: 4}, at)
	if !tr.Estimate || !reflect.DeepEqual(tr.Month, trafficWindow{From: "2026-10-01", RX: 86400, TX: 129600, Days: 1, Samples: 4}) {
		t.Fatal(tr)
	}
	if tr.Last30d.Days != 30 || tr.Last30d.DaysMissing != 29 || tr.Last30d.From != "2026-09-02" || tr.Last30d.RX != 86400 {
		t.Fatal(tr)
	}
	days := []dayRate{{Day: "2026-09-01", RX: ptr(99), TX: ptr(99), Samples: 999}, {Day: "2026-09-02", RX: ptr(1), TX: ptr(2), Samples: 10}, {Day: "2026-09-30", RX: nil, TX: ptr(4), Samples: 20}}
	tr = trafficEstimate(days, &partial{RX: ptr(-2), TX: nil, Samples: 3}, at)
	if tr.Last30d.RX != 86400 || tr.Last30d.TX != 518400 || tr.Last30d.Samples != 33 || tr.Last30d.DaysMissing != 29 {
		t.Fatal(tr)
	}
	if tr.Month.RX != 0 || tr.Month.TX != 0 || tr.Month.DaysMissing != 1 || tr.Month.Samples != 3 {
		t.Fatal(tr)
	}
	tr = trafficEstimate(nil, nil, at)
	if tr.Month.DaysMissing != 1 || tr.Last30d.DaysMissing != 30 || tr.Month.RX != 0 || tr.Month.Samples != 0 {
		t.Fatal(tr)
	}
	tr = trafficEstimate(nil, &partial{Samples: 7}, at)
	if tr.Month.DaysMissing != 1 || tr.Month.Samples != 7 {
		t.Fatal(tr)
	}
	tr = trafficEstimate(nil, &partial{RX: ptr(0.5), TX: ptr(0)}, at.Add(-12*time.Hour+time.Second))
	if tr.Month.RX != 1 || tr.Month.TX != 0 || tr.Month.DaysMissing != 0 {
		t.Fatal(tr)
	}
}

func TestTrafficEstimateAtUTCMidnight(t *testing.T) {
	at := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name             string
		today            *partial
		missing, samples int
	}{
		{"absent", nil, 1, 0},
		{"all null", &partial{Samples: 1}, 1, 1},
		{"valid", &partial{RX: ptr(2), TX: ptr(3), Samples: 1}, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := trafficEstimate(nil, tc.today, at)
			if got.Month.RX != 0 || got.Month.TX != 0 || got.Month.Days != 1 || got.Month.DaysMissing != tc.missing || got.Month.Samples != tc.samples || got.Last30d.DaysMissing != 29+tc.missing {
				t.Fatal(got)
			}
		})
	}
}

func TestIndexTrafficReadsDailyAndToday(t *testing.T) {
	public := t.TempDir()
	at := time.Date(2027, 1, 1, 12, 0, 0, 0, time.UTC)
	g := Group{ID: "net-fixture", Kind: "net", Name: "fixture", Columns: []string{"time", "rx", "tx"}, Units: []string{"s", "bytes/s", "bytes/s"}}
	day := at.AddDate(0, 0, -1).Truncate(24 * time.Hour)
	for _, r := range [][]any{{day.Unix(), 2, 3}, {day.Unix() + 60, 4, 5}} {
		if e := AppendGroup(public, g, r[0].(int64), r); e != nil {
			t.Fatal(e)
		}
	}
	if e := Rollup(public, at); e != nil {
		t.Fatal(e)
	}
	for _, r := range [][]any{{at.Unix() - 60, 2, nil}, {at.Unix(), 4, 6}, {at.Unix(), 6, 8}, {at.Unix() + 1, 999, 999}} {
		if e := AppendGroup(public, g, r[0].(int64), r); e != nil {
			t.Fatal(e)
		}
	}
	if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: at.Unix()}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(public, "index.json"))
	var idx Index
	if e := json.Unmarshal(b, &idx); e != nil {
		t.Fatal(e)
	}
	if len(idx.Groups) != 1 || idx.Groups[0].Traffic == nil {
		t.Fatal(string(b))
	}
	tr := idx.Groups[0].Traffic
	if tr.Month.RX != 172800 || tr.Month.TX != 345600 || tr.Month.Samples != 2 || tr.Month.DaysMissing != 0 {
		t.Fatal(tr)
	}
	if tr.Last30d.RX != 432000 || tr.Last30d.TX != 691200 || tr.Last30d.Samples != 4 || tr.Last30d.DaysMissing != 28 {
		t.Fatal(tr)
	}
}

func TestMalformedObjectDiscardsOnlyThatLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "cpu.jsonl")
	h, _ := json.Marshal(testGroup().Header())
	if e := os.WriteFile(p, append(h, []byte("\n[1790812800,2]\n{broken}\n[1790812860,4]\n")...), 0644); e != nil {
		t.Fatal(e)
	}
	_, r, e := readMinuteDay(p)
	if e != nil || len(r) != 2 || *r[1].Values["user"] != 4 {
		t.Fatal(r, e)
	}
}

func TestIndexTrafficIncludesThirtyFirstDayOfMonth(t *testing.T) {
	public := t.TempDir()
	at := time.Date(2026, 10, 31, 12, 0, 0, 0, time.UTC)
	g := Group{ID: "net-fixture", Kind: "net", Name: "fixture", Columns: []string{"time", "rx", "tx"}, Units: []string{"s", "bytes/s", "bytes/s"}}
	h := g.Header()
	h.Step = 86400
	h.Columns, h.Units, _ = coarseColumns(g.Columns, g.Units)
	start := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC).Unix()
	if e := appendRows(filepath.Join(public, "data/1d/2026", g.ID+".jsonl"), h, [][]any{{start, 1, start + 60, 2, 2, 3, 3}}); e != nil {
		t.Fatal(e)
	}
	if e := WriteIndex(public, Index{Schema: 1, GeneratedAt: at.Unix()}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(filepath.Join(public, "index.json"))
	var idx Index
	json.Unmarshal(b, &idx)
	if len(idx.Groups) != 1 || idx.Groups[0].Traffic == nil {
		t.Fatal(string(b))
	}
	tr := idx.Groups[0].Traffic
	if tr.Month.RX != 172800 || tr.Month.TX != 259200 || tr.Month.Days != 31 || tr.Month.DaysMissing != 30 || tr.Month.Samples != 1 || tr.Last30d.RX != 0 || tr.Last30d.DaysMissing != 30 {
		t.Fatal(tr)
	}
}
