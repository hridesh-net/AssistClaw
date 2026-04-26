package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/assistclaw/assistclaw/internal/email"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

func emailCmd(gf *globalFlags) *cobra.Command {
	root := &cobra.Command{
		Use:   "email",
		Short: "Autonomous email assistant (OAuth, pending drafts, approvals)",
	}
	root.AddCommand(emailLoginGmailCmd(gf))
	root.AddCommand(emailLoginGraphCmd(gf))
	root.AddCommand(emailAccountsCmd(gf))
	root.AddCommand(emailRulesCmd(gf))
	root.AddCommand(emailPendingCmd(gf))
	root.AddCommand(emailApproveCmd(gf))
	root.AddCommand(emailRejectCmd(gf))
	root.AddCommand(emailEditCmd(gf))
	root.AddCommand(emailStatusCmd(gf))
	root.AddCommand(emailDoctorCmd(gf))
	return root
}

func emailAccountsCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "accounts",
		Short: "List configured email.accounts from config",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			if len(cfg.Email.Accounts) == 0 {
				fmt.Println("No email.accounts in config.")
				return nil
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "NAME\tBACKEND")
			for _, a := range cfg.Email.Accounts {
				fmt.Fprintf(w, "%s\t%s\n", a.Name, a.Backend)
			}
			return w.Flush()
		},
	}
	return cmd
}

func emailRulesCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rules",
		Short: "List rules for each email account in config",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			for _, a := range cfg.Email.Accounts {
				fmt.Printf("Account %q (%s):\n", a.Name, a.Backend)
				if len(a.Rules) == 0 {
					fmt.Println("  (no rules — defaults to auto for all mail)")
					continue
				}
				for i, r := range a.Rules {
					fmt.Printf("  [%d] action=%s match=%+v\n", i, r.Action, r.Match)
				}
			}
			return nil
		},
	}
	return cmd
}

func emailDoctorCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run email-related checks (same as assistclaw doctor subset for email.*)",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, usedPath, cfgIssues, err := loadConfigForDoctor(gf.configPath)
			if err != nil {
				return err
			}
			checks := doctorChecksFromConfigIssues(usedPath, cfgIssues)
			checks = append(checks, checkEmailAssistant(cfg)...)
			for _, c := range checks {
				fmt.Printf("[%s] %s: %s\n", c.Severity, c.ID, c.Message)
			}
			return nil
		},
	}
	return cmd
}

func emailLoginGmailCmd(gf *globalFlags) *cobra.Command {
	var account string
	cmd := &cobra.Command{
		Use:   "login-gmail",
		Short: "OAuth browser login for Gmail API; saves token JSON under state dir",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			if strings.TrimSpace(account) == "" {
				return fmt.Errorf("--account is required")
			}
			clientID := os.Getenv("ASSISTCLAW_GMAIL_CLIENT_ID")
			clientSecret := os.Getenv("ASSISTCLAW_GMAIL_CLIENT_SECRET")
			if clientID == "" {
				return fmt.Errorf("set ASSISTCLAW_GMAIL_CLIENT_ID (and optional ASSISTCLAW_GMAIL_CLIENT_SECRET)")
			}
			oauthCfg := &oauth2.Config{
				ClientID:     clientID,
				ClientSecret: clientSecret,
				RedirectURL:  "http://127.0.0.1:18791/oauth/callback",
				Scopes: []string{
					"https://www.googleapis.com/auth/gmail.readonly",
					"https://www.googleapis.com/auth/gmail.send",
				},
				Endpoint: google.Endpoint,
			}
			// Minimal localhost redirect: user copies ?code= from browser if redirect fails
			authURL := oauthCfg.AuthCodeURL("assistclaw", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
			fmt.Println("Open this URL in a browser, sign in, then paste the authorization code:")
			fmt.Println(authURL)
			fmt.Print("Authorization code: ")
			var code string
			if _, err := fmt.Scanln(&code); err != nil {
				return err
			}
			ctx := context.Background()
			tok, err := oauthCfg.Exchange(ctx, strings.TrimSpace(code))
			if err != nil {
				return err
			}
			outDir := filepath.Join(cfg.StateDir, "email")
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			outPath := filepath.Join(outDir, "oauth-"+account+"-gmail.json")
			raw, _ := json.MarshalIndent(tok, "", "  ")
			if err := os.WriteFile(outPath, raw, 0o600); err != nil {
				return err
			}
			fmt.Printf("Saved token to %s\nSet email.accounts[].gmail.token_file: email/oauth-%s-gmail.json\n", outPath, account)
			return nil
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "account name matching email.accounts[].name")
	return cmd
}

func emailLoginGraphCmd(gf *globalFlags) *cobra.Command {
	var account string
	cmd := &cobra.Command{
		Use:   "login-graph",
		Short: "OAuth browser login for Microsoft Graph mail",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			if strings.TrimSpace(account) == "" {
				return fmt.Errorf("--account is required")
			}
			clientID := os.Getenv("ASSISTCLAW_GRAPH_CLIENT_ID")
			clientSecret := os.Getenv("ASSISTCLAW_GRAPH_CLIENT_SECRET")
			if clientID == "" {
				return fmt.Errorf("set ASSISTCLAW_GRAPH_CLIENT_ID (and ASSISTCLAW_GRAPH_CLIENT_SECRET for confidential clients)")
			}
			oauthCfg := &oauth2.Config{
				ClientID:     clientID,
				ClientSecret: clientSecret,
				RedirectURL:  "http://127.0.0.1:18791/oauth/callback",
				Scopes: []string{
					"https://graph.microsoft.com/Mail.Read",
					"https://graph.microsoft.com/Mail.Send",
					"offline_access",
				},
				Endpoint: oauth2.Endpoint{
					AuthURL:  "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
					TokenURL: "https://login.microsoftonline.com/common/oauth2/v2.0/token",
				},
			}
			authURL := oauthCfg.AuthCodeURL("assistclaw", oauth2.AccessTypeOffline, oauth2.ApprovalForce)
			fmt.Println("Open this URL, sign in, then paste the authorization code:")
			fmt.Println(authURL)
			fmt.Print("Authorization code: ")
			var code string
			if _, err := fmt.Scanln(&code); err != nil {
				return err
			}
			ctx := context.Background()
			tok, err := oauthCfg.Exchange(ctx, strings.TrimSpace(code))
			if err != nil {
				return err
			}
			outDir := filepath.Join(cfg.StateDir, "email")
			if err := os.MkdirAll(outDir, 0o755); err != nil {
				return err
			}
			outPath := filepath.Join(outDir, "oauth-"+account+"-graph.json")
			raw, _ := json.MarshalIndent(tok, "", "  ")
			if err := os.WriteFile(outPath, raw, 0o600); err != nil {
				return err
			}
			fmt.Printf("Saved token to %s\nSet email.accounts[].graph.token_file: email/oauth-%s-graph.json\n", outPath, account)
			return nil
		},
	}
	cmd.Flags().StringVar(&account, "account", "", "account name")
	return cmd
}

func emailPendingCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "pending",
		Short: "List pending mail draft approval tokens",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			st, err := email.OpenStore(cfg.StateDir)
			if err != nil {
				return err
			}
			defer st.Close()
			ctx := context.Background()
			rows, err := st.ListPendingDrafts(ctx)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				fmt.Println("No pending drafts.")
				return nil
			}
			for _, r := range rows {
				fmt.Printf("%s  account=%s  subject=%q  at=%s\n", r.Token, r.Account, r.Subject, r.CreatedAt.Format("2006-01-02 15:04"))
			}
			return nil
		},
	}
}

func emailApproveCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "approve TOKEN",
		Short: "Approve and send a pending draft (same as channel command)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEmailCLIAction(gf, "approve", args[0], "")
		},
	}
}

func emailRejectCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "reject TOKEN",
		Short: "Reject a pending draft",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEmailCLIAction(gf, "reject", args[0], "")
		},
	}
}

func emailEditCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "edit TOKEN NEW_BODY...",
		Short: "Replace draft body and mark pending",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			body := strings.Join(args[1:], " ")
			return runEmailCLIAction(gf, "edit", args[0], body)
		},
	}
}

func emailStatusCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether email assistant is enabled in config",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			fmt.Printf("email.enabled=%v accounts=%d notify.channel=%q notify.session_id=%q\n",
				cfg.Email.Enabled, len(cfg.Email.Accounts), cfg.Email.Notify.Channel, cfg.Email.Notify.SessionID)
			return nil
		},
	}
}

func runEmailCLIAction(gf *globalFlags, verb, token, editBody string) error {
	log := buildLogger(gf.logLevel)
	defer log.Sync() //nolint:errcheck
	cfg, err := loadConfig(gf.configPath, log)
	if err != nil {
		return err
	}
	if !cfg.Email.Enabled {
		return fmt.Errorf("email.enabled is false in config")
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
	if svc == nil {
		return fmt.Errorf("email assistant is disabled in config")
	}
	ctx := context.Background()
	var text string
	switch verb {
	case "approve":
		text = "approve " + token
	case "reject":
		text = "reject " + token
	case "edit":
		text = "edit " + token + ": " + editBody
	default:
		return fmt.Errorf("unknown verb")
	}
	reply, handled, err := svc.HandleInboundCommand(ctx, cfg.Email.Notify.Channel, cfg.Email.Notify.SessionID, text)
	if err != nil {
		return err
	}
	if !handled {
		return fmt.Errorf("command not handled (check notify session matches)")
	}
	if reply != "" {
		fmt.Println(reply)
	}
	return nil
}
