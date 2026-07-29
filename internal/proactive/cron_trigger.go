package proactive

import (
	"context"
	"fmt"

	"github.com/robfig/cron/v3"
)

// CronTrigger emits events on a cron schedule.
// Each schedule entry maps to a rule ID so the engine can route it.
type CronTrigger struct {
	name     string
	schedule string
	payload  map[string]any
}

// NewCronTrigger creates a trigger that fires on the given cron schedule.
func NewCronTrigger(name, schedule string, payload map[string]any) *CronTrigger {
	if payload == nil {
		payload = make(map[string]any)
	}
	return &CronTrigger{
		name:     name,
		schedule: schedule,
		payload:  payload,
	}
}

// Name returns the trigger identifier.
func (c *CronTrigger) Name() string { return c.name }

// Start runs the cron scheduler and emits events on tick.
func (c *CronTrigger) Start(ctx context.Context, emit EmitFunc) error {
	cr := cron.New(cron.WithSeconds())
	_, err := cr.AddFunc(c.schedule, func() {
		emit(Event{
			Source:  c.name,
			Type:    "tick",
			Payload: c.payload,
		})
	})
	if err != nil {
		return fmt.Errorf("invalid cron schedule %q: %w", c.schedule, err)
	}
	cr.Start()
	<-ctx.Done()
	cr.Stop()
	return ctx.Err()
}
