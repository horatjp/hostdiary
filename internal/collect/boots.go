package collect

import "hostdiary/internal/metrics"

// The last saved entry belongs to the previous observation's boot. Keep it for
// the same boot regardless of age; apply the window only once it becomes past.
// Past boots keep 1095 days, independently of the three daily year files.
func bootHistory(prev State, cur metrics.Observation, at int64) []int64 {
	same := cur.BootID != "" && cur.BootID == prev.Observation.BootID
	known := same && len(prev.Boots) > 0
	var current int64
	if known {
		current = prev.Boots[len(prev.Boots)-1]
	} else if cur.BootID != "" && cur.UptimeValid {
		current = at - int64(cur.Uptime)
		known = true
	}
	boots := []int64{}
	for i, b := range prev.Boots {
		if same && i == len(prev.Boots)-1 {
			continue
		}
		if b >= at-1095*86400 {
			boots = append(boots, b)
		}
	}
	if known {
		boots = append(boots, current)
	}
	return boots
}
