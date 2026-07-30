// Package proactive implements the event-driven rule engine that transforms
// AssistClaw from a reactive agent into a proactive Jarvis.
//
// Architecture: Trigger → Event → Engine → Rule (match + cooldown) → Action → Notifier
package proactive

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"
)

// Event is the unit of flow through the engine.
type Event struct {
	Source  string         // trigger name (e.g. "email", "cron", "manual")
	Type    string         // domain event (e.g. "received", "starting_soon")
	Payload map[string]any // structured data available to templates
	Time    time.Time
}

// EmitFunc is the callback triggers use to inject events.
type EmitFunc func(Event)

// Trigger produces events. Each trigger owns its goroutine and lifecycle.
type Trigger interface {
	Name() string
	Start(ctx context.Context, emit EmitFunc) error
}

// Predicate filters events before a rule fires.
type Predicate func(Event) bool

// Rule defines when and how to react to an event.
type Rule struct {
	ID       string
	Trigger  string        // must match Event.Source
	Match    Predicate     // optional additional filter
	Action   string        // registered action name
	Prompt   string        // text/template against Event; passed to action
	NotifyTo []string      // registered notifier names
	Cooldown time.Duration // minimum time between firings
	Enabled  bool
}

// Action executes the business logic for a matched rule.
type Action interface {
	Name() string
	Execute(ctx context.Context, ev Event, rule Rule) (string, error)
}

// Notification carries the result of an action to a user-facing channel.
type Notification struct {
	RuleID string
	Body   string
	Meta   map[string]string // links, action buttons, severity
}

// Notifier delivers notifications to external channels (Telegram, Discord, console, etc.).
type Notifier interface {
	Name() string
	Send(ctx context.Context, n Notification) error
}

// CompilePrompt renders a rule's Prompt template against an event.
func CompilePrompt(promptTmpl string, ev Event) (string, error) {
	return compilePrompt(promptTmpl, ev)
}

// compilePrompt is the internal implementation.
func compilePrompt(promptTmpl string, ev Event) (string, error) {
	if promptTmpl == "" {
		return "", nil
	}
	tmpl, err := template.New("rule").Parse(promptTmpl)
	if err != nil {
		return "", fmt.Errorf("parse prompt template: %w", err)
	}
	var b bytes.Buffer
	if err := tmpl.Execute(&b, ev); err != nil {
		return "", fmt.Errorf("execute prompt template: %w", err)
	}
	return b.String(), nil
}
