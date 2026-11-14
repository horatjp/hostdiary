// Traffic estimates deliberately extrapolate each day's observed mean over its elapsed seconds.
package store

import (
	"math"
	"time"
)

type partial struct {
	RX, TX  *float64
	Samples int
}
type dayRate struct {
	Day     string
	RX, TX  *float64
	Samples int
}
type trafficWindow struct {
	From        string `json:"from"`
	RX          int64  `json:"rx"`
	TX          int64  `json:"tx"`
	Days        int    `json:"days"`
	DaysMissing int    `json:"days_missing"`
	Samples     int    `json:"samples"`
}
type traffic struct {
	Estimate    bool          `json:"estimate"`
	Description string        `json:"description"`
	Month       trafficWindow `json:"month"`
	Last30d     trafficWindow `json:"last30d"`
}

func trafficEstimate(days []dayRate, today *partial, at time.Time) traffic {
	at = at.UTC()
	start := at.Truncate(24 * time.Hour)
	rates := map[string]dayRate{}
	for _, d := range days {
		rates[d.Day] = d
	}
	window := func(from time.Time) trafficWindow {
		out := trafficWindow{From: from.Format("2006-01-02")}
		var rx, tx float64
		for d := from; !d.After(start); d = d.AddDate(0, 0, 1) {
			out.Days++
			r, ok := rates[d.Format("2006-01-02")]
			duration := float64(86400)
			if d.Equal(start) {
				ok = today != nil
				r = dayRate{}
				duration = at.Sub(start).Seconds()
				if ok {
					r.RX = today.RX
					r.TX = today.TX
					r.Samples = today.Samples
				}
			}
			if !ok || r.RX == nil || r.TX == nil {
				out.DaysMissing++
			}
			if ok {
				out.Samples += r.Samples
			}
			if r.RX != nil {
				rx += math.Max(0, *r.RX) * duration
			}
			if r.TX != nil {
				tx += math.Max(0, *r.TX) * duration
			}
		}
		out.RX = int64(math.Round(rx))
		out.TX = int64(math.Round(tx))
		return out
	}
	return traffic{true, "Traffic estimate: missing time is extrapolated from each day's mean; samples counts 1-minute rows, including null values; compare with the elapsed minutes.", window(time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)), window(start.AddDate(0, 0, -29))}
}
