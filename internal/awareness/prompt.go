package awareness

import (
	"fmt"
	"strings"
	"time"
)

// friendlyNames maps well-known signal keys to readable labels. Unknown keys
// render with the raw key so external signals (phone, sensors) still surface.
var friendlyNames = map[string]string{
	"calendar.next_event":    "Next calendar event",
	"calendar.current_event": "Current calendar event",
	"user.activity":          "User presence",
	"user.location":          "User location",
	"device.battery":         "Phone battery",
	"device.network":         "Phone network",
}

// PromptBlock renders the live signals as a system-prompt section. Returns ""
// when there is nothing useful to say, so callers can skip the section.
func (s *Store) PromptBlock() string {
	now := time.Now()
	snap := s.Snapshot()

	var b strings.Builder
	b.WriteString("## Live Context (your awareness of the present moment)\n")
	b.WriteString("These signals describe the user's situation right now. Let them shape tone, urgency, and suggestions naturally — do not recite them back unless asked.\n")
	fmt.Fprintf(&b, "- Local time: %s (%s)\n", now.Format("Mon Jan 2 15:04"), Daypart(now))

	for _, key := range s.Keys() {
		sig := snap[key]
		label, ok := friendlyNames[key]
		if !ok {
			label = key
		}
		age := now.Sub(sig.UpdatedAt)
		freshness := ""
		if age > 2*time.Minute {
			freshness = fmt.Sprintf(" (as of %s ago)", age.Round(time.Minute))
		}
		fmt.Fprintf(&b, "- %s: %s%s\n", label, sig.Value, freshness)
	}
	return b.String()
}
