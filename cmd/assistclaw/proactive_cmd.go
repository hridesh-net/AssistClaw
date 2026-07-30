package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/assistclaw/assistclaw/internal/proactive"
	"github.com/spf13/cobra"
)

func proactiveCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "proactive",
		Short: "Jarvis proactive engine commands",
		Long:  `Control and test the event-driven proactive rule engine.`,
	}
	cmd.AddCommand(proactiveNotifyCmd(gf))
	cmd.AddCommand(proactiveTestCmd(gf))
	return cmd
}

func proactiveNotifyCmd(gf *globalFlags) *cobra.Command {
	var channel string
	cmd := &cobra.Command{
		Use:   "notify [message]",
		Short: "Send a proactive notification to a channel",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			_, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			msg := args[0]
			if channel == "" {
				return fmt.Errorf("--channel is required")
			}

			// Build a minimal engine with a console notifier.
			eng := proactive.NewEngine(log)
			eng.RegisterNotifier(proactive.NewWriterNotifier(channel, os.Stdout))

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			eng.Start(ctx)
			defer eng.Stop()

			n, ok := eng.Notifier(channel)
			if !ok {
				return fmt.Errorf("unknown notifier %q", channel)
			}
			return n.Send(ctx, proactive.Notification{
				RuleID: "cli",
				Body:   msg,
				Meta: map[string]string{
					"channel": channel,
					"source":  "cli",
				},
			})
		},
	}
	cmd.Flags().StringVarP(&channel, "channel", "c", "", "Target notifier name (e.g. telegram, discord, console)")
	return cmd
}

func proactiveTestCmd(gf *globalFlags) *cobra.Command {
	var (
		ruleID    string
		eventJSON string
	)
	cmd := &cobra.Command{
		Use:   "test",
		Short: "Test a rule by injecting a synthetic event",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			if ruleID == "" {
				return fmt.Errorf("--rule is required")
			}

			// Build a minimal engine for dry-run testing.
			eng := proactive.NewEngine(log)
			eng.RegisterTrigger(proactive.NewManualTrigger())
			eng.RegisterNotifier(proactive.NewWriterNotifier("console", os.Stdout))
			// Use a mock action that echoes the compiled prompt so we can test the full
			// template compilation + dispatch path without a full agent runner.
			eng.RegisterAction(&echoAction{})

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			eng.Start(ctx)
			defer eng.Stop()

			rulesPath := filepath.Join(cfg.StateDir, "rules.yaml")
			var rules []proactive.Rule
			if r, err := eng.LoadRulesFromYAML(rulesPath); err == nil {
				rules = r
				if err := eng.SetRules(rules); err != nil {
					return fmt.Errorf("set rules: %w", err)
				}
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("load rules: %w", err)
			}

			var targetRule *proactive.Rule
			for _, r := range rules {
				if r.ID == ruleID {
					targetRule = &r
					break
				}
			}
			if targetRule == nil {
				return fmt.Errorf("rule %q not found", ruleID)
			}

			var ev proactive.Event
			if err := json.Unmarshal([]byte(eventJSON), &ev); err != nil {
				return fmt.Errorf("parse event JSON: %w", err)
			}
			if ev.Source == "" && targetRule.Trigger != "" {
				ev.Source = targetRule.Trigger
			}
			if ev.Time.IsZero() {
				ev.Time = time.Now()
			}

			prompt, result, err := eng.TestRule(ctx, ev, *targetRule)
			if err != nil {
				return fmt.Errorf("rule test failed: %w", err)
			}

			fmt.Printf("Rule:      %s\n", targetRule.ID)
			fmt.Printf("Trigger:   %s\n", targetRule.Trigger)
			fmt.Printf("Action:    %s\n", targetRule.Action)
			fmt.Printf("Prompt:    %s\n", prompt)
			fmt.Printf("Result:    %s\n", result)
			fmt.Printf("NotifyTo:  %v\n", targetRule.NotifyTo)
			return nil
		},
	}
	cmd.Flags().StringVar(&ruleID, "rule", "", "Rule ID to test")
	cmd.Flags().StringVar(&eventJSON, "event", "{}", "Synthetic event payload as JSON")
	return cmd
}

// echoAction is a test seam that returns the compiled prompt unchanged.
type echoAction struct{}

func (a *echoAction) Name() string { return "run_agent" }

func (a *echoAction) Execute(_ context.Context, _ proactive.Event, rule proactive.Rule) (string, error) {
	return rule.Prompt, nil
}
