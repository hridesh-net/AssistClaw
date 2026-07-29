package proactive

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// CalendarEvent represents a calendar event for polling.
type CalendarEvent struct {
	ID        string
	Title     string
	StartTime time.Time
	Attendees []string
}

// CalendarSource is the interface the trigger uses to fetch events.
type CalendarSource interface {
	ListUpcoming(ctx context.Context, from, to time.Time) ([]CalendarEvent, error)
}

// CalendarTrigger polls a calendar source and emits events as meetings approach.
type CalendarTrigger struct {
	name       string
	source     CalendarSource
	interval   time.Duration
	warnBefore time.Duration
	log        *zap.Logger
	// fired tracks event IDs we've already emitted for a given window.
	fired map[string]time.Time
}

// NewCalendarTrigger creates a calendar polling trigger.
func NewCalendarTrigger(name string, source CalendarSource, interval, warnBefore time.Duration, log *zap.Logger) *CalendarTrigger {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	if warnBefore <= 0 {
		warnBefore = 10 * time.Minute
	}
	if log == nil {
		log = zap.NewNop()
	}
	return &CalendarTrigger{
		name:       name,
		source:     source,
		interval:   interval,
		warnBefore: warnBefore,
		log:        log,
		fired:      make(map[string]time.Time),
	}
}

// Name returns the trigger identifier.
func (c *CalendarTrigger) Name() string { return "calendar:" + c.name }

// Start polls the calendar and emits starting_soon events.
func (c *CalendarTrigger) Start(ctx context.Context, emit EmitFunc) error {
	ticker := time.NewTicker(c.interval)
	defer ticker.Stop()

	c.poll(ctx, emit)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			c.poll(ctx, emit)
		}
	}
}

func (c *CalendarTrigger) poll(ctx context.Context, emit EmitFunc) {
	now := time.Now()
	windowEnd := now.Add(c.warnBefore + c.interval)

	events, err := c.source.ListUpcoming(ctx, now, windowEnd)
	if err != nil {
		c.log.Warn("calendar poll failed", zap.String("calendar", c.name), zap.Error(err))
		return
	}

	// Clean up old fired entries.
	for id, t := range c.fired {
		if now.Sub(t) > c.warnBefore+c.interval {
			delete(c.fired, id)
		}
	}

	for _, ev := range events {
		remaining := ev.StartTime.Sub(now)
		if remaining < 0 || remaining > c.warnBefore {
			continue
		}
		if _, ok := c.fired[ev.ID]; ok {
			continue
		}
		c.fired[ev.ID] = now

		emit(Event{
			Source: c.Name(),
			Type:   "starting_soon",
			Payload: map[string]any{
				"event_id":   ev.ID,
				"title":      ev.Title,
				"start_time": ev.StartTime,
				"attendees":  ev.Attendees,
				"minutes":    int(remaining.Minutes()),
			},
			Time: now,
		})
	}
}
