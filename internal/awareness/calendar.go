package awareness

import (
	"context"
	"fmt"
	"time"
)

// CalendarEvent mirrors the proactive package's event shape without importing it.
type CalendarEvent struct {
	ID        string
	Title     string
	StartTime time.Time
	Attendees []string
}

// ListUpcomingFunc fetches events in a window; main.go adapts the configured
// calendar source (e.g. Google Calendar) to this signature.
type ListUpcomingFunc func(ctx context.Context, from, to time.Time) ([]CalendarEvent, error)

// StartCalendarFeed keeps "calendar.next_event" fresh by polling the source.
// Errors leave the previous signal in place until its TTL lapses.
func StartCalendarFeed(ctx context.Context, store *Store, list ListUpcomingFunc, interval time.Duration) {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			updateNextEvent(pollCtx, store, list, interval)
			cancel()
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
}

func updateNextEvent(ctx context.Context, store *Store, list ListUpcomingFunc, interval time.Duration) {
	now := time.Now()
	events, err := list(ctx, now, now.Add(12*time.Hour))
	if err != nil {
		return
	}
	var next *CalendarEvent
	for i := range events {
		ev := &events[i]
		if ev.StartTime.Before(now) {
			continue
		}
		if next == nil || ev.StartTime.Before(next.StartTime) {
			next = ev
		}
	}
	if next == nil {
		store.Set("calendar.next_event", "nothing scheduled in the next 12 hours", 2*interval)
		return
	}
	in := next.StartTime.Sub(now).Round(time.Minute)
	val := fmt.Sprintf("%q at %s (in %s)", next.Title, next.StartTime.Format("15:04"), in)
	if len(next.Attendees) > 0 {
		val += fmt.Sprintf(", with %d attendee(s)", len(next.Attendees))
	}
	store.Set("calendar.next_event", val, 2*interval)
}
