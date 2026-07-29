package proactive

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// RuleYAML is the on-disk representation of a rule.
type RuleYAML struct {
	ID       string            `yaml:"id"`
	Trigger  string            `yaml:"trigger"`
	Match    map[string]any    `yaml:"match,omitempty"`
	Action   string            `yaml:"action"`
	Prompt   string            `yaml:"prompt"`
	NotifyTo []string          `yaml:"notify_to"`
	Cooldown string            `yaml:"cooldown"`
	Enabled  *bool             `yaml:"enabled,omitempty"`
}

// RulesFile is the top-level YAML document.
type RulesFile struct {
	Rules []RuleYAML `yaml:"rules"`
}

// LoadRulesFromYAML reads and parses a rules YAML file, converting to runtime Rules.
func LoadRulesFromYAML(path string, triggers map[string]Trigger, actions map[string]Action, notifiers map[string]Notifier) ([]Rule, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read rules file: %w", err)
	}

	var file RulesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse rules file: %w", err)
	}

	rules := make([]Rule, 0, len(file.Rules))
	for i, ry := range file.Rules {
		if ry.ID == "" {
			return nil, fmt.Errorf("rule at index %d missing id", i)
		}
		if ry.Trigger != "" {
			if _, ok := triggers[ry.Trigger]; !ok {
				return nil, fmt.Errorf("rule %q references unknown trigger %q", ry.ID, ry.Trigger)
			}
		}
		if ry.Action != "" {
			if _, ok := actions[ry.Action]; !ok {
				return nil, fmt.Errorf("rule %q references unknown action %q", ry.ID, ry.Action)
			}
		}
		for _, n := range ry.NotifyTo {
			if _, ok := notifiers[n]; !ok {
				return nil, fmt.Errorf("rule %q references unknown notifier %q", ry.ID, n)
			}
		}

		var cooldown time.Duration
		if ry.Cooldown != "" {
			var err error
			cooldown, err = time.ParseDuration(ry.Cooldown)
			if err != nil {
				return nil, fmt.Errorf("rule %q invalid cooldown %q: %w", ry.ID, ry.Cooldown, err)
			}
		}

		enabled := true
		if ry.Enabled != nil {
			enabled = *ry.Enabled
		}

		// Build a simple predicate from Match if provided.
		var pred Predicate
		if len(ry.Match) > 0 {
			pred = buildMatchPredicate(ry.Match)
		}

		rules = append(rules, Rule{
			ID:       ry.ID,
			Trigger:  ry.Trigger,
			Match:    pred,
			Action:   ry.Action,
			Prompt:   ry.Prompt,
			NotifyTo: ry.NotifyTo,
			Cooldown: cooldown,
			Enabled:  enabled,
		})
	}
	return rules, nil
}

// buildMatchPredicate creates a predicate that checks Event.Payload keys.
func buildMatchPredicate(match map[string]any) Predicate {
	return func(ev Event) bool {
		for k, want := range match {
			got, ok := ev.Payload[k]
			if !ok {
				return false
			}
			if got != want {
				return false
			}
		}
		return true
	}
}
