package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"

	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	"github.com/assistclaw/assistclaw/internal/proactive"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func ruleCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rule",
		Short: "Manage proactive rules",
		Long:  `List, add, enable, disable, and test proactive rules.`,
	}
	cmd.AddCommand(ruleListCmd(gf))
	cmd.AddCommand(ruleAddCmd(gf))
	cmd.AddCommand(ruleEnableCmd(gf))
	cmd.AddCommand(ruleDisableCmd(gf))
	cmd.AddCommand(ruleTestCmd(gf))
	return cmd
}

func rulesPath(cfgPath string) string {
	// Derive rules file from config path or default state dir.
	if cfgPath != "" {
		return filepath.Join(filepath.Dir(cfgPath), "rules.yaml")
	}
	return filepath.Join(os.Getenv("HOME"), ".assistclaw", "rules.yaml")
}

func ruleListCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all proactive rules",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := rulesPath(gf.configPath)
			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Println("No rules.yaml found.")
					return nil
				}
				return err
			}

			var file proactive.RulesFile
			if err := yaml.Unmarshal(data, &file); err != nil {
				return fmt.Errorf("parse rules: %w", err)
			}

			prim := func(s string) string { return tui.Style(tui.ColorPrimary, true, s) }
			dim := func(s string) string { return tui.Style(tui.ColorMuted, false, s) }
			header := func(s string) string { return tui.Style(tui.ColorNeon, true, s) }

			fmt.Println(header("\n📋 Proactive Rules") + dim(fmt.Sprintf("  (%d rules)", len(file.Rules))))
			fmt.Println(dim("─────────────────────────────────────────────────────────────"))

			if len(file.Rules) == 0 {
				fmt.Println(dim("  No rules configured."))
				return nil
			}

			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n",
				dim("ID"), dim("TRIGGER"), dim("ACTION"), dim("NOTIFY"), dim("STATUS"))
			for _, r := range file.Rules {
				status := dim("enabled")
				if r.Enabled != nil && !*r.Enabled {
					status = prim("disabled")
				}
				fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n",
					prim(r.ID), r.Trigger, r.Action, fmt.Sprintf("%v", r.NotifyTo), status)
			}
			w.Flush()
			fmt.Println()
			return nil
		},
	}
}

func ruleAddCmd(gf *globalFlags) *cobra.Command {
	var (
		trigger  string
		action   string
		prompt   string
		notifyTo []string
		cooldown string
	)
	cmd := &cobra.Command{
		Use:   "add [id]",
		Short: "Add a new proactive rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := rulesPath(gf.configPath)
			var file proactive.RulesFile

			if data, err := os.ReadFile(path); err == nil {
				_ = yaml.Unmarshal(data, &file)
			}

			// Check for duplicate ID.
			for _, r := range file.Rules {
				if r.ID == args[0] {
					return fmt.Errorf("rule %q already exists", args[0])
				}
			}

			enabled := true
			file.Rules = append(file.Rules, proactive.RuleYAML{
				ID:       args[0],
				Trigger:  trigger,
				Action:   action,
				Prompt:   prompt,
				NotifyTo: notifyTo,
				Cooldown: cooldown,
				Enabled:  &enabled,
			})

			data, err := yaml.Marshal(file)
			if err != nil {
				return err
			}
			if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
				return err
			}
			if err := os.WriteFile(path, data, 0644); err != nil {
				return err
			}
			fmt.Printf("✅ Rule %q added.\n", args[0])
			return nil
		},
	}
	cmd.Flags().StringVarP(&trigger, "trigger", "t", "", "Trigger name (required)")
	cmd.Flags().StringVarP(&action, "action", "a", "run_agent", "Action name")
	cmd.Flags().StringVarP(&prompt, "prompt", "p", "", "Prompt template")
	cmd.Flags().StringArrayVarP(&notifyTo, "notify", "n", nil, "Notifier names")
	cmd.Flags().StringVar(&cooldown, "cooldown", "5m", "Cooldown duration")
	_ = cmd.MarkFlagRequired("trigger")
	return cmd
}

func ruleEnableCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "enable [id]",
		Short: "Enable a rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setRuleEnabled(gf.configPath, args[0], true)
		},
	}
}

func ruleDisableCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "disable [id]",
		Short: "Disable a rule",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return setRuleEnabled(gf.configPath, args[0], false)
		},
	}
}

func setRuleEnabled(configPath, id string, enabled bool) error {
	path := rulesPath(configPath)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file proactive.RulesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return err
	}
	found := false
	for i := range file.Rules {
		if file.Rules[i].ID == id {
			file.Rules[i].Enabled = &enabled
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("rule %q not found", id)
	}
	out, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		return err
	}
	status := "enabled"
	if !enabled {
		status = "disabled"
	}
	fmt.Printf("✅ Rule %q %s.\n", id, status)
	return nil
}

func ruleTestCmd(gf *globalFlags) *cobra.Command {
	var eventJSON string
	cmd := &cobra.Command{
		Use:   "test [id]",
		Short: "Test a rule with a synthetic event",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := rulesPath(gf.configPath)
			data, err := os.ReadFile(path)
			if err != nil {
				if os.IsNotExist(err) {
					return fmt.Errorf("no rules.yaml found at %s", path)
				}
				return err
			}
			var file proactive.RulesFile
			if err := yaml.Unmarshal(data, &file); err != nil {
				return fmt.Errorf("parse rules: %w", err)
			}

			var targetRule *proactive.RuleYAML
			for i := range file.Rules {
				if file.Rules[i].ID == args[0] {
					targetRule = &file.Rules[i]
					break
				}
			}
			if targetRule == nil {
				return fmt.Errorf("rule %q not found", args[0])
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

			prompt, err := proactive.CompilePrompt(targetRule.Prompt, ev)
			if err != nil {
				return fmt.Errorf("compile prompt: %w", err)
			}

			fmt.Printf("Rule:     %s\n", targetRule.ID)
			fmt.Printf("Trigger:  %s\n", targetRule.Trigger)
			fmt.Printf("Action:   %s\n", targetRule.Action)
			fmt.Printf("Prompt:\n%s\n", prompt)
			fmt.Printf("NotifyTo: %v\n", targetRule.NotifyTo)
			return nil
		},
	}
	cmd.Flags().StringVar(&eventJSON, "event", "{}", "Synthetic event JSON payload")
	return cmd
}
