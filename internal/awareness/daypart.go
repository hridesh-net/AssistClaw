package awareness

import "time"

// Daypart maps a local time to a coarse persona mode. The boundaries are
// deliberately simple; per-user schedules can override via the
// "user.daypart_override" signal.
func Daypart(t time.Time) string {
	switch h := t.Hour(); {
	case h >= 5 && h < 9:
		return "early morning"
	case h >= 9 && h < 12:
		return "morning, work hours"
	case h >= 12 && h < 17:
		return "afternoon, work hours"
	case h >= 17 && h < 21:
		return "evening"
	case h >= 21 || h < 1:
		return "late evening, winding down"
	default:
		return "deep night — only urgent matters deserve attention"
	}
}
