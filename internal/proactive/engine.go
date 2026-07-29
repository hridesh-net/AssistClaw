package proactive

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// Engine implements the core rule matching, cooldown enforcement, and dispatch loop.
type Engine struct {
	log *zap.Logger

	// registries
	triggers  map[string]Trigger
	actions   map[string]Action
	notifiers map[string]Notifier

	// rules are swapped atomically via the mutex.
	mu    sync.RWMutex
	rules []Rule

	// cooldown tracks the last fired time per rule ID.
	cooldownMu sync.RWMutex
	cooldown   map[string]time.Time

	// lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// inbound events from all triggers
	events chan Event
}

// NewEngine creates an empty proactive engine.
func NewEngine(log *zap.Logger) *Engine {
	if log == nil {
		log = zap.NewNop()
	}
	return &Engine{
		log:       log,
		triggers:  make(map[string]Trigger),
		actions:   make(map[string]Action),
		notifiers: make(map[string]Notifier),
		cooldown:  make(map[string]time.Time),
		events:    make(chan Event, 256),
	}
}

// RegisterTrigger adds a trigger to the engine. Must be called before Start.
func (e *Engine) RegisterTrigger(t Trigger) {
	e.triggers[t.Name()] = t
}

// RegisterAction adds an action to the engine. Must be called before Start.
func (e *Engine) RegisterAction(a Action) {
	e.actions[a.Name()] = a
}

// RegisterNotifier adds a notifier to the engine. Must be called before Start.
func (e *Engine) RegisterNotifier(n Notifier) {
	e.notifiers[n.Name()] = n
}

// Notifier returns a registered notifier by name.
func (e *Engine) Notifier(name string) (Notifier, bool) {
	n, ok := e.notifiers[name]
	return n, ok
}

// SetRules atomically replaces the active rule set. Invalid references return an error
// and leave the old rules intact.
func (e *Engine) SetRules(rules []Rule) error {
	// validate references
	for _, r := range rules {
		if r.Trigger != "" {
			if _, ok := e.triggers[r.Trigger]; !ok {
				return fmt.Errorf("rule %q references unknown trigger %q", r.ID, r.Trigger)
			}
		}
		if r.Action != "" {
			if _, ok := e.actions[r.Action]; !ok {
				return fmt.Errorf("rule %q references unknown action %q", r.ID, r.Action)
			}
		}
		for _, n := range r.NotifyTo {
			if _, ok := e.notifiers[n]; !ok {
				return fmt.Errorf("rule %q references unknown notifier %q", r.ID, n)
			}
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.rules = append([]Rule(nil), rules...)
	return nil
}

// Rules returns a snapshot of the currently active rules.
func (e *Engine) Rules() []Rule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]Rule(nil), e.rules...)
}

// LoadRulesFromYAML loads and validates rules from a YAML file against the
// engine's current trigger/action/notifier registries.
func (e *Engine) LoadRulesFromYAML(path string) ([]Rule, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return LoadRulesFromYAML(path, e.triggers, e.actions, e.notifiers)
}

// TestRule synchronously evaluates a single rule against an event without
// enforcing cooldowns or sending notifications. It returns the compiled
// prompt and the action result (if the action exists).
func (e *Engine) TestRule(ctx context.Context, ev Event, rule Rule) (string, string, error) {
	prompt, err := compilePrompt(rule.Prompt, ev)
	if err != nil {
		return "", "", fmt.Errorf("compile prompt: %w", err)
	}
	var result string
	if act, ok := e.actions[rule.Action]; ok {
		testRule := rule
		testRule.Prompt = prompt
		res, err := act.Execute(ctx, ev, testRule)
		if err != nil {
			return prompt, "", fmt.Errorf("action failed: %w", err)
		}
		result = res
	}
	return prompt, result, nil
}

// IsRunning reports whether the engine has been started and not yet stopped.
func (e *Engine) IsRunning() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.ctx != nil && e.ctx.Err() == nil
}

// Start launches all registered triggers and begins the dispatch loop.
func (e *Engine) Start(ctx context.Context) {
	e.ctx, e.cancel = context.WithCancel(ctx)

	// Spawn triggers
	for name, t := range e.triggers {
		e.wg.Add(1)
		go func(n string, tr Trigger) {
			defer e.wg.Done()
			if err := tr.Start(e.ctx, e.emit); err != nil {
				e.log.Warn("trigger exited", zap.String("trigger", n), zap.Error(err))
			}
		}(name, t)
	}

	// Dispatch loop
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		for {
			select {
			case <-e.ctx.Done():
				return
			case ev := <-e.events:
				e.handleEvent(ev)
			}
		}
	}()

	e.log.Info("proactive engine started", zap.Int("triggers", len(e.triggers)), zap.Int("actions", len(e.actions)), zap.Int("notifiers", len(e.notifiers)))
}

// Stop signals all triggers and the dispatch loop to shut down, then waits for clean exit.
func (e *Engine) Stop() {
	if e.cancel != nil {
		e.cancel()
	}
	e.wg.Wait()
	e.log.Info("proactive engine stopped")
}

// emit is the callback used by triggers to enqueue events.
func (e *Engine) emit(ev Event) {
	if ev.Time.IsZero() {
		ev.Time = time.Now()
	}
	select {
	case e.events <- ev:
	case <-e.ctx.Done():
	}
}

// handleEvent matches the event against rules and dispatches actions + notifications.
func (e *Engine) handleEvent(ev Event) {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if rule.Trigger != "" && rule.Trigger != ev.Source {
			continue
		}
		if rule.Match != nil && !rule.Match(ev) {
			continue
		}
		if e.onCooldown(rule) {
			e.log.Debug("rule suppressed by cooldown",
				zap.String("rule", rule.ID),
				zap.String("trigger", ev.Source),
			)
			continue
		}

		e.fireRule(ev, rule)
	}
}

// onCooldown reports whether the rule is still within its cooldown window.
func (e *Engine) onCooldown(rule Rule) bool {
	if rule.Cooldown <= 0 {
		return false
	}
	e.cooldownMu.RLock()
	last, ok := e.cooldown[rule.ID]
	e.cooldownMu.RUnlock()
	return ok && time.Since(last) < rule.Cooldown
}

// fireRule executes the rule's action and fans out notifications.
func (e *Engine) fireRule(ev Event, rule Rule) {
	// Record cooldown immediately to prevent re-entrant storms.
	e.cooldownMu.Lock()
	e.cooldown[rule.ID] = time.Now()
	e.cooldownMu.Unlock()

	ctx, cancel := context.WithTimeout(e.ctx, 5*time.Minute)
	defer cancel()

	// Compile prompt template
	prompt, err := compilePrompt(rule.Prompt, ev)
	if err != nil {
		e.log.Warn("failed to compile rule prompt",
			zap.String("rule", rule.ID),
			zap.Error(err),
		)
		// Continue with empty prompt rather than dropping the event entirely.
	}

	// Override the rule's prompt with the rendered version for the action.
	rule.Prompt = prompt

	// Execute action
	var actionResult string
	if act, ok := e.actions[rule.Action]; ok {
		res, err := act.Execute(ctx, ev, rule)
		if err != nil {
			e.log.Warn("action failed",
				zap.String("rule", rule.ID),
				zap.String("action", rule.Action),
				zap.Error(err),
			)
			actionResult = fmt.Sprintf("Action error: %v", err)
		} else {
			actionResult = res
		}
	} else if rule.Action != "" {
		e.log.Warn("action not found", zap.String("rule", rule.ID), zap.String("action", rule.Action))
		actionResult = fmt.Sprintf("Action %q not found", rule.Action)
	}

	// Fan out notifications
	notif := Notification{
		RuleID: rule.ID,
		Body:   actionResult,
		Meta: map[string]string{
			"trigger":   ev.Source,
			"event_type": ev.Type,
		},
	}
	for _, name := range rule.NotifyTo {
		if n, ok := e.notifiers[name]; ok {
			if err := n.Send(ctx, notif); err != nil {
				e.log.Warn("notifier failed",
					zap.String("rule", rule.ID),
					zap.String("notifier", name),
					zap.Error(err),
				)
			}
		}
	}
}
