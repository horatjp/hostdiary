package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"time"
)

func readTraffic(public, id string, at time.Time) (traffic, error) {
	at = at.UTC()
	today := at.Truncate(24 * time.Hour)
	from := today.AddDate(0, 0, -29)
	monthStart := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
	if monthStart.Before(from) {
		from = monthStart // On the 31st, this month begins before the last-30-day window.
	}
	days := []dayRate{}
	// The two windows together never need more than two UTC years.
	for year := from.Year(); year <= at.Year(); year++ {
		path := filepath.Join(public, "data/1d", time.Date(year, 1, 1, 0, 0, 0, 0, time.UTC).Format("2006"), id+".jsonl")
		rows, e := readDailyRates(path, from, today)
		if e != nil {
			return traffic{}, e
		}
		days = append(days, rows...)
	}
	path := filepath.Join(public, "data/1m", today.Format("2006-01-02"), id+".jsonl")
	_, rows, e := readMinuteDay(path)
	if os.IsNotExist(e) {
		e = nil
	}
	if e != nil {
		return traffic{}, e
	}
	unique := map[int64]namedRow{}
	for _, r := range rows {
		if r.Time >= today.Unix() && r.Time <= at.Unix() {
			unique[r.Time] = r
		}
	}
	var partialDay *partial
	if len(unique) > 0 {
		p := partial{Samples: len(unique)}
		counts := map[string]int{}
		sums := map[string]float64{}
		for _, r := range unique {
			for _, c := range []string{"rx", "tx"} {
				if v := r.Values[c]; v != nil {
					sums[c] += *v
					counts[c]++
				}
			}
		}
		if counts["rx"] > 0 {
			v := sums["rx"] / float64(counts["rx"])
			p.RX = &v
		}
		if counts["tx"] > 0 {
			v := sums["tx"] / float64(counts["tx"])
			p.TX = &v
		}
		partialDay = &p
	}
	return trafficEstimate(days, partialDay, at), nil
}

func readDailyRates(path string, from, today time.Time) ([]dayRate, error) {
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	defer f.Close()
	reader := bufio.NewReader(f)
	var h Header
	out := []dayRate{}
	for {
		b, e := reader.ReadBytes('\n')
		if e == io.EOF {
			break
		}
		if e != nil {
			return nil, e
		}
		b = bytes.TrimSpace(b)
		if len(b) == 0 {
			continue
		}
		if b[0] == '{' {
			var next Header
			if json.Unmarshal(b, &next) == nil {
				h = next
			}
			continue
		}
		if h.Schema != 1 || h.Step != 86400 || len(h.Columns) == 0 || h.Columns[0] != "time" {
			continue
		}
		var raw []json.RawMessage
		if json.Unmarshal(b, &raw) != nil || len(raw) != len(h.Columns) {
			continue
		}
		var t int64
		if json.Unmarshal(raw[0], &t) != nil || t%86400 != 0 || t < from.Unix() || t >= today.Unix() {
			continue
		}
		d := dayRate{Day: time.Unix(t, 0).UTC().Format("2006-01-02")}
		valid := true
		for i, c := range h.Columns {
			switch c {
			case "rx":
				if json.Unmarshal(raw[i], &d.RX) != nil {
					valid = false
				}
			case "tx":
				if json.Unmarshal(raw[i], &d.TX) != nil {
					valid = false
				}
			case "samples":
				if json.Unmarshal(raw[i], &d.Samples) != nil {
					valid = false
				}
			}
		}
		if valid {
			out = append(out, d)
		}
	}
	return out, nil
}
