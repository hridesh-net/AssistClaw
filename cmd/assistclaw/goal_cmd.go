package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	"github.com/assistclaw/assistclaw/internal/email"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

func goalCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "goal",
		Short: "Goal-driven email threads",
		Long: `Give AssistClaw an email objective and it manages the whole thread:
drafts the opening mail, processes replies, follows up on silence, and reports
when the objective is achieved. Every send still requires your approval
(approve TOKEN in the notify channel, or: assistclaw email approve TOKEN).`,
	}
	cmd.AddCommand(goalAddCmd(gf))
	cmd.AddCommand(goalListCmd(gf))
	cmd.AddCommand(goalShowCmd(gf))
	cmd.AddCommand(goalCancelCmd(gf))
	return cmd
}

func openGoalStore(gf *globalFlags) (*email.Store, func(), error) {
	log := buildLogger(gf.logLevel)
	cfg, err := loadConfig(gf.configPath, log)
	if err != nil {
		return nil, nil, err
	}
	st, err := email.OpenStore(cfg.StateDir)
	if err != nil {
		return nil, nil, err
	}
	return st, func() { _ = st.Close(); _ = log.Sync() }, nil
}

func goalAddCmd(gf *globalFlags) *cobra.Command {
	var account, to, subject, followupAfter string
	var maxFollowups int
	c := &cobra.Command{
		Use:   "add OBJECTIVE...",
		Short: "Open a new email goal (drafts the opening mail for approval)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			objective := strings.TrimSpace(strings.Join(args, " "))
			if objective == "" {
				return fmt.Errorf("objective is required")
			}
			if to == "" || subject == "" {
				return fmt.Errorf("--to and --subject are required")
			}
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			if !cfg.Email.Enabled {
				return fmt.Errorf("email.enabled is false in config")
			}
			if account == "" {
				if len(cfg.Email.Accounts) != 1 {
					return fmt.Errorf("--account is required (multiple accounts configured)")
				}
				account = cfg.Email.Accounts[0].Name
			}
			fa := 48 * time.Hour
			if followupAfter != "" {
				d, err := time.ParseDuration(followupAfter)
				if err != nil {
					return fmt.Errorf("--followup-after: %w", err)
				}
				fa = d
			}
			st, err := email.OpenStore(cfg.StateDir)
			if err != nil {
				return err
			}
			defer st.Close()
			reg := provider.NewRegistry()
			if err := registerProviders(context.Background(), cfg, reg, log); err != nil {
				log.Warn("providers", zap.Error(err))
			}
			model := cfg.Email.Model
			if model == "" {
				model = cfg.Routing.Default
			}
			p, _, err := reg.ResolveModel(context.Background(), model)
			if err != nil {
				return err
			}
			svc, err := email.NewService(cfg, st, p, model, nil, nil, log)
			if err != nil {
				return err
			}
			g, token, draft, err := svc.OpenGoal(context.Background(), account, to, subject, objective, fa, maxFollowups)
			if err != nil {
				return err
			}
			prim := func(s string) string { return tui.Style(tui.ColorPrimary, true, s) }
			dim := func(s string) string { return tui.Style(tui.ColorMuted, false, s) }
			fmt.Printf("\n%s  goal #%d opened (%s → %s)\n", prim("🎯"), g.ID, g.AccountName, g.Counterpart)
			fmt.Printf("%s\n\n", dim("objective: "+g.Objective))
			fmt.Printf("opening draft:\n%s\n\n", draft)
			fmt.Printf("approve with:  %s\n", prim("assistclaw email approve "+token))
			fmt.Printf("or in your notify channel:  approve %s | edit %s: <body> | reject %s\n", token, token, token)
			return nil
		},
	}
	c.Flags().StringVar(&account, "account", "", "email account name (default: the only configured account)")
	c.Flags().StringVar(&to, "to", "", "counterpart email address (required)")
	c.Flags().StringVar(&subject, "subject", "", "thread subject (required)")
	c.Flags().StringVar(&followupAfter, "followup-after", "48h", "nudge the counterpart after this much silence")
	c.Flags().IntVar(&maxFollowups, "max-followups", 3, "maximum number of follow-up nudges")
	return c
}

func goalListCmd(gf *globalFlags) *cobra.Command {
	var all bool
	c := &cobra.Command{
		Use:   "list",
		Short: "List goals",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, done, err := openGoalStore(gf)
			if err != nil {
				return err
			}
			defer done()
			goals, err := st.ListGoals(context.Background(), !all)
			if err != nil {
				return err
			}
			prim := func(s string) string { return tui.Style(tui.ColorPrimary, true, s) }
			dim := func(s string) string { return tui.Style(tui.ColorMuted, false, s) }
			header := func(s string) string { return tui.Style(tui.ColorNeon, true, s) }
			fmt.Println(header("\n🎯 Email Goals") + dim(fmt.Sprintf("  (%d)", len(goals))))
			fmt.Println(dim("─────────────────────────────────────────────────────────────"))
			if len(goals) == 0 {
				fmt.Println(dim("  No goals. Open one with: assistclaw goal add --to ... --subject ... \"objective\""))
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\n",
				dim("ID"), dim("STATUS"), dim("WITH"), dim("OBJECTIVE"), dim("FOLLOW-UPS"), dim("LAST ACTIVITY"))
			for _, g := range goals {
				obj := g.Objective
				if len(obj) > 48 {
					obj = obj[:48] + "…"
				}
				fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%d/%d\t%s\n",
					prim(strconv.FormatInt(g.ID, 10)), string(g.Status), g.Counterpart, obj,
					g.FollowupsSent, g.MaxFollowups, g.LastActivity.Format("Jan 2 15:04"))
			}
			return w.Flush()
		},
	}
	c.Flags().BoolVar(&all, "all", false, "include achieved/abandoned goals")
	return c
}

func goalShowCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "show ID",
		Short: "Show a goal's full history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid goal id %q", args[0])
			}
			st, done, err := openGoalStore(gf)
			if err != nil {
				return err
			}
			defer done()
			ctx := context.Background()
			g, err := st.GetGoal(ctx, id)
			if err != nil {
				return fmt.Errorf("goal %d not found", id)
			}
			events, err := st.ListGoalEvents(ctx, id, 200)
			if err != nil {
				return err
			}
			prim := func(s string) string { return tui.Style(tui.ColorPrimary, true, s) }
			dim := func(s string) string { return tui.Style(tui.ColorMuted, false, s) }
			fmt.Printf("\n%s goal #%d — %s\n", prim("🎯"), g.ID, g.Objective)
			fmt.Printf("%s\n", dim(fmt.Sprintf("status: %s | account: %s | with: %s | subject: %s",
				g.Status, g.AccountName, g.Counterpart, g.Subject)))
			fmt.Printf("%s\n\n", dim(fmt.Sprintf("follow-ups: %d/%d every %s | opened: %s",
				g.FollowupsSent, g.MaxFollowups, g.FollowupAfter, g.CreatedAt.Format("Jan 2 15:04"))))
			for _, e := range events {
				fmt.Printf("%s %s\n", dim(e.CreatedAt.Format("Jan 2 15:04")), prim(e.Kind))
				if d := strings.TrimSpace(e.Detail); d != "" {
					for _, line := range strings.Split(d, "\n") {
						fmt.Printf("    %s\n", line)
					}
				}
			}
			return nil
		},
	}
}

func goalCancelCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "cancel ID",
		Short: "Abandon a goal (stops replies and follow-ups)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid goal id %q", args[0])
			}
			st, done, err := openGoalStore(gf)
			if err != nil {
				return err
			}
			defer done()
			ctx := context.Background()
			g, err := st.GetGoal(ctx, id)
			if err != nil {
				return fmt.Errorf("goal %d not found", id)
			}
			if !g.Status.IsOpen() {
				return fmt.Errorf("goal %d is already %s", id, g.Status)
			}
			if err := st.SetGoalStatus(ctx, id, email.GoalAbandoned, "cancelled by user"); err != nil {
				return err
			}
			fmt.Printf("Goal #%d abandoned. Pending drafts for it can still be rejected with their tokens.\n", id)
			return nil
		},
	}
}
