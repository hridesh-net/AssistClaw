package proactive

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// fakeInvoker is a test double for AgentInvoker.
type fakeInvoker struct {
	calls atomic.Int32
	mu    sync.Mutex
	last  string
}

func (f *fakeInvoker) Run(_ context.Context, prompt string) (string, error) {
	f.calls.Add(1)
	f.mu.Lock()
	f.last = prompt
	f.mu.Unlock()
	return fmt.Sprintf("result: %s", prompt), nil
}

func TestEngine_StartStop_Clean(t *testing.T) {
	eng := NewEngine(zap.NewNop())
	mt := NewManualTrigger()
	eng.RegisterTrigger(mt)

	ctx, cancel := context.WithCancel(context.Background())
	eng.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	time.Sleep(50 * time.Millisecond)

	cancel()
	eng.Stop()
}

func TestEngine_ManualFire_RunAgent_WriterNotifier(t *testing.T) {
	var buf bytes.Buffer
	inv := &fakeInvoker{}

	eng := NewEngine(zap.NewNop())
	mt := NewManualTrigger()
	eng.RegisterTrigger(mt)
	eng.RegisterAction(NewRunAgentAction(inv))
	notif := NewWriterNotifier("console", &buf)
	eng.RegisterNotifier(notif)

	if err := eng.SetRules([]Rule{
		{
			ID:       "test-rule",
			Trigger:  "manual",
			Action:   "run_agent",
			Prompt:   "handle {{.Source}} event",
			NotifyTo: []string{"console"},
			Enabled:  true,
		},
	}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	eng.Start(ctx)

	// The trigger goroutine captures the emit callback asynchronously after
	// Start; retry until the event is accepted instead of silently dropping it.
	fireDeadline := time.Now().Add(2 * time.Second)
	for !mt.Fire(Event{Source: "manual", Type: "ping", Payload: map[string]any{"x": 1}}) {
		if time.Now().After(fireDeadline) {
			t.Fatal("timed out waiting for manual trigger to arm")
		}
		time.Sleep(5 * time.Millisecond)
	}

	deadline := time.Now().Add(2 * time.Second)
	for inv.calls.Load() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for agent invocation")
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	eng.Stop()

	inv.mu.Lock()
	lastPrompt := inv.last
	inv.mu.Unlock()
	if lastPrompt != "handle manual event" {
		t.Fatalf("expected prompt %q, got %q", "handle manual event", lastPrompt)
	}
	if !strings.Contains(buf.String(), "result: handle manual event") {
		t.Fatalf("expected notifier output to contain result, got: %s", buf.String())
	}
}

// fakeTrigger is a test double with a configurable name.
type fakeTrigger struct {
	name string
	mu   sync.Mutex
	emit EmitFunc
}

func (f *fakeTrigger) Name() string { return f.name }
func (f *fakeTrigger) Start(_ context.Context, emit EmitFunc) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.emit = emit
	return nil
}
func (f *fakeTrigger) Fire(ev Event) {
	f.mu.Lock()
	emit := f.emit
	f.mu.Unlock()
	if emit != nil {
		emit(ev)
	}
}

func TestEngine_RuleMatch_FilterByTrigger(t *testing.T) {
	var buf bytes.Buffer
	inv := &fakeInvoker{}

	eng := NewEngine(zap.NewNop())
	manual := NewManualTrigger()
	email := &fakeTrigger{name: "email"}

	eng.RegisterTrigger(manual)
	eng.RegisterTrigger(email)
	eng.RegisterAction(NewRunAgentAction(inv))
	eng.RegisterNotifier(NewWriterNotifier("console", &buf))

	if err := eng.SetRules([]Rule{
		{
			ID:       "email-rule",
			Trigger:  "email",
			Action:   "run_agent",
			NotifyTo: []string{"console"},
			Enabled:  true,
		},
	}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	eng.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	defer func() {
		cancel()
		eng.Stop()
	}()

	// Fire from "manual" trigger — should not match "email" rule.
	manual.Fire(Event{Source: "manual", Type: "ping"})
	time.Sleep(200 * time.Millisecond)
	if inv.calls.Load() != 0 {
		t.Fatalf("expected 0 calls, got %d", inv.calls.Load())
	}

	// Fire from "email" trigger — should match.
	email.Fire(Event{Source: "email", Type: "received"})
	deadline := time.Now().Add(2 * time.Second)
	for inv.calls.Load() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for agent invocation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEngine_RuleMatch_Predicate(t *testing.T) {
	var buf bytes.Buffer
	inv := &fakeInvoker{}

	eng := NewEngine(zap.NewNop())
	mt := NewManualTrigger()
	eng.RegisterTrigger(mt)
	eng.RegisterAction(NewRunAgentAction(inv))
	eng.RegisterNotifier(NewWriterNotifier("console", &buf))

	if err := eng.SetRules([]Rule{
		{
			ID:      "important-only",
			Trigger: "manual",
			Match: func(ev Event) bool {
				imp, _ := ev.Payload["important"].(bool)
				return imp
			},
			Action:   "run_agent",
			NotifyTo: []string{"console"},
			Enabled:  true,
		},
	}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	eng.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	defer func() {
		cancel()
		eng.Stop()
	}()

	// Non-important event should be dropped by predicate.
	mt.Fire(Event{Source: "manual", Type: "ping", Payload: map[string]any{"important": false}})
	time.Sleep(200 * time.Millisecond)
	if inv.calls.Load() != 0 {
		t.Fatalf("expected 0 calls, got %d", inv.calls.Load())
	}

	// Important event should fire.
	mt.Fire(Event{Source: "manual", Type: "ping", Payload: map[string]any{"important": true}})
	deadline := time.Now().Add(2 * time.Second)
	for inv.calls.Load() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for agent invocation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEngine_Cooldown(t *testing.T) {
	var buf bytes.Buffer
	inv := &fakeInvoker{}

	eng := NewEngine(zap.NewNop())
	mt := NewManualTrigger()
	eng.RegisterTrigger(mt)
	eng.RegisterAction(NewRunAgentAction(inv))
	eng.RegisterNotifier(NewWriterNotifier("console", &buf))

	if err := eng.SetRules([]Rule{
		{
			ID:       "cool-rule",
			Trigger:  "manual",
			Action:   "run_agent",
			NotifyTo: []string{"console"},
			Cooldown: 500 * time.Millisecond,
			Enabled:  true,
		},
	}); err != nil {
		t.Fatalf("set rules: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	eng.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	defer func() {
		cancel()
		eng.Stop()
	}()

	// First fire should succeed.
	mt.Fire(Event{Source: "manual", Type: "ping"})
	deadline := time.Now().Add(2 * time.Second)
	for inv.calls.Load() != 1 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for first agent invocation")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Immediate second fire should be suppressed by cooldown.
	mt.Fire(Event{Source: "manual", Type: "ping"})
	time.Sleep(200 * time.Millisecond)
	if inv.calls.Load() != 1 {
		t.Fatalf("expected 1 call after cooldown suppression, got %d", inv.calls.Load())
	}

	// After cooldown expires, fire should succeed again.
	time.Sleep(400 * time.Millisecond)
	mt.Fire(Event{Source: "manual", Type: "ping"})
	deadline = time.Now().Add(2 * time.Second)
	for inv.calls.Load() != 2 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for second agent invocation")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestEngine_SetRules_Validation(t *testing.T) {
	eng := NewEngine(zap.NewNop())
	eng.RegisterTrigger(NewManualTrigger())
	eng.RegisterAction(NewRunAgentAction(&fakeInvoker{}))
	eng.RegisterNotifier(NewWriterNotifier("console", &bytes.Buffer{}))

	// Valid rules should apply.
	if err := eng.SetRules([]Rule{
		{ID: "r1", Trigger: "manual", Action: "run_agent", NotifyTo: []string{"console"}},
	}); err != nil {
		t.Fatalf("valid rules should apply: %v", err)
	}

	// Unknown trigger should error and leave old rules intact.
	if err := eng.SetRules([]Rule{
		{ID: "r2", Trigger: "unknown", Action: "run_agent"},
	}); err == nil {
		t.Fatal("expected error for unknown trigger")
	} else if !strings.Contains(err.Error(), "unknown trigger") {
		t.Fatalf("expected 'unknown trigger' in error, got: %v", err)
	}
	rules := eng.Rules()
	if len(rules) != 1 || rules[0].ID != "r1" {
		t.Fatalf("old rules should be intact, got: %+v", rules)
	}

	// Unknown action should error.
	if err := eng.SetRules([]Rule{
		{ID: "r3", Trigger: "manual", Action: "unknown"},
	}); err == nil {
		t.Fatal("expected error for unknown action")
	} else if !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("expected 'unknown action' in error, got: %v", err)
	}

	// Unknown notifier should error.
	if err := eng.SetRules([]Rule{
		{ID: "r4", Trigger: "manual", Action: "run_agent", NotifyTo: []string{"unknown"}},
	}); err == nil {
		t.Fatal("expected error for unknown notifier")
	} else if !strings.Contains(err.Error(), "unknown notifier") {
		t.Fatalf("expected 'unknown notifier' in error, got: %v", err)
	}
}

func TestEngine_ConcurrentRuleUpdates(t *testing.T) {
	eng := NewEngine(zap.NewNop())
	mt := NewManualTrigger()
	eng.RegisterTrigger(mt)
	eng.RegisterAction(NewRunAgentAction(&fakeInvoker{}))
	eng.RegisterNotifier(NewWriterNotifier("console", &bytes.Buffer{}))

	ctx, cancel := context.WithCancel(context.Background())
	eng.Start(ctx)
	time.Sleep(100 * time.Millisecond)
	defer func() {
		cancel()
		eng.Stop()
	}()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = eng.SetRules([]Rule{
				{ID: fmt.Sprintf("r-%d", idx), Trigger: "manual", Action: "run_agent", NotifyTo: []string{"console"}},
			})
		}(i)
	}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mt.Fire(Event{Source: "manual", Type: "ping"})
		}()
	}
	wg.Wait()
	// The test passes if the race detector finds no issues.
}
