package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/email"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"gopkg.in/yaml.v3"
)

func emailCmd(gf *globalFlags) *cobra.Command {
	root := &cobra.Command{
		Use:   "email",
		Short: "Autonomous email assistant (OAuth, pending drafts, approvals)",
	}
	root.AddCommand(emailLoginGmailCmd(gf))
	root.AddCommand(emailLoginGraphCmd(gf))
	root.AddCommand(emailSetupCmd(gf))
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

type emailSetupOptions struct {
	Account        string
	Backend        string
	NotifyChannel  string
	NotifySession  string
	FromChannel    string
	IMAPHost       string
	IMAPUser       string
	IMAPPass       string
	SMTPHost       string
	SMTPPort       int
	SMTPUser       string
	SMTPPass       string
	NonInteractive bool
	RunLogin       bool
}

func emailSetupCmd(gf *globalFlags) *cobra.Command {
	opts := &emailSetupOptions{
		SMTPPort: 587,
		RunLogin: true,
	}
	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Interactive email setup wizard (writes config + optional OAuth login)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !opts.NonInteractive {
				if err := promptEmailSetup(opts); err != nil {
					return err
				}
			}
			if strings.TrimSpace(opts.Account) == "" {
				return fmt.Errorf("account name is required")
			}
			backend := strings.ToLower(strings.TrimSpace(opts.Backend))
			if backend != "imap" && backend != "gmail" && backend != "graph" {
				return fmt.Errorf("backend must be one of: imap, gmail, graph")
			}
			cfgPath := gf.configPath
			if cfgPath == "" {
				cfgPath = config.DefaultConfigPath()
			}
			cfg, err := loadOrInitConfigForSetup(cfgPath)
			if err != nil {
				return err
			}
			if strings.TrimSpace(opts.NotifySession) == "" && strings.TrimSpace(opts.FromChannel) != "" {
				fromCh := strings.ToLower(strings.TrimSpace(opts.FromChannel))
				if strings.TrimSpace(opts.NotifyChannel) == "" {
					opts.NotifyChannel = fromCh
				}
				if !strings.EqualFold(opts.NotifyChannel, fromCh) {
					return fmt.Errorf("--notify-channel and --from-channel must match when both are set")
				}
				autoSession, err := detectNotifySessionFromHistory(cfg, fromCh)
				if err != nil {
					return fmt.Errorf("auto-detect notify session for %q: %w", fromCh, err)
				}
				opts.NotifySession = autoSession
				fmt.Printf("Detected notify session from recent history: %s\n", autoSession)
			}
			if strings.TrimSpace(opts.NotifyChannel) == "" || strings.TrimSpace(opts.NotifySession) == "" {
				return fmt.Errorf("notify channel and session are required")
			}
			acc, err := accountConfigFromSetup(opts)
			if err != nil {
				return err
			}
			cfg.Email.Enabled = true
			cfg.Email.Notify = config.EmailNotifyConfig{
				Channel:   strings.ToLower(strings.TrimSpace(opts.NotifyChannel)),
				SessionID: strings.TrimSpace(opts.NotifySession),
			}
			cfg.Email.Accounts = upsertEmailAccount(cfg.Email.Accounts, acc)
			if err := writeConfigForSetup(cfgPath, cfg); err != nil {
				return err
			}
			fmt.Printf("Updated email config in %s (account=%s backend=%s)\n", cfgPath, acc.Name, acc.Backend)
			if !opts.RunLogin {
				fmt.Println("Skipped OAuth login. Run `assistclaw email login-gmail --account ...` or `assistclaw email login-graph --account ...` later if needed.")
				return nil
			}
			switch backend {
			case "gmail":
				return runEmailOAuthLoginGmail(gf, acc.Name)
			case "graph":
				return runEmailOAuthLoginGraph(gf, acc.Name)
			default:
				return nil
			}
		},
	}
	cmd.Flags().StringVar(&opts.Account, "account", "", "email account name")
	cmd.Flags().StringVar(&opts.Backend, "backend", "", "backend: imap | gmail | graph")
	cmd.Flags().StringVar(&opts.NotifyChannel, "notify-channel", "", "channel to publish drafts: telegram | slack | discord")
	cmd.Flags().StringVar(&opts.NotifySession, "notify-session", "", "session id for notify channel (e.g. tg:123)")
	cmd.Flags().StringVar(&opts.FromChannel, "from-channel", "", "auto-detect notify session from recent chat history for this channel (telegram|slack|discord)")
	cmd.Flags().StringVar(&opts.IMAPHost, "imap-host", "", "IMAP host:port (required for imap backend)")
	cmd.Flags().StringVar(&opts.IMAPUser, "imap-user", "", "IMAP username")
	cmd.Flags().StringVar(&opts.IMAPPass, "imap-pass", "", "IMAP password or app password")
	cmd.Flags().StringVar(&opts.SMTPHost, "smtp-host", "", "SMTP host")
	cmd.Flags().IntVar(&opts.SMTPPort, "smtp-port", 587, "SMTP port")
	cmd.Flags().StringVar(&opts.SMTPUser, "smtp-user", "", "SMTP username")
	cmd.Flags().StringVar(&opts.SMTPPass, "smtp-pass", "", "SMTP password")
	cmd.Flags().BoolVar(&opts.NonInteractive, "non-interactive", false, "require all values from flags; disable prompts")
	cmd.Flags().BoolVar(&opts.RunLogin, "run-login", true, "run OAuth login immediately for gmail/graph")
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

func promptEmailSetup(opts *emailSetupOptions) error {
	in := bufio.NewReader(os.Stdin)
	prompt := func(label, current string) (string, error) {
		if strings.TrimSpace(current) != "" {
			return current, nil
		}
		fmt.Print(label)
		v, err := in.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(v), nil
	}
	var err error
	opts.Account, err = prompt("Account name: ", opts.Account)
	if err != nil {
		return err
	}
	opts.Backend, err = prompt("Backend [imap|gmail|graph]: ", opts.Backend)
	if err != nil {
		return err
	}
	opts.NotifyChannel, err = prompt("Notify channel [telegram|slack|discord]: ", opts.NotifyChannel)
	if err != nil {
		return err
	}
	opts.NotifySession, err = prompt("Notify session id (e.g. tg:123): ", opts.NotifySession)
	if err != nil {
		return err
	}
	if strings.EqualFold(strings.TrimSpace(opts.Backend), "imap") {
		opts.IMAPHost, err = prompt("IMAP host:port: ", opts.IMAPHost)
		if err != nil {
			return err
		}
		opts.IMAPUser, err = prompt("IMAP username: ", opts.IMAPUser)
		if err != nil {
			return err
		}
		opts.IMAPPass, err = prompt("IMAP password/app password: ", opts.IMAPPass)
		if err != nil {
			return err
		}
		opts.SMTPHost, err = prompt("SMTP host: ", opts.SMTPHost)
		if err != nil {
			return err
		}
		port, err := prompt(fmt.Sprintf("SMTP port [%d]: ", opts.SMTPPort), "")
		if err != nil {
			return err
		}
		if strings.TrimSpace(port) != "" {
			p, convErr := strconv.Atoi(strings.TrimSpace(port))
			if convErr != nil {
				return fmt.Errorf("invalid smtp port %q: %w", port, convErr)
			}
			opts.SMTPPort = p
		}
		opts.SMTPUser, err = prompt("SMTP username: ", opts.SMTPUser)
		if err != nil {
			return err
		}
		opts.SMTPPass, err = prompt("SMTP password: ", opts.SMTPPass)
		if err != nil {
			return err
		}
	}
	return nil
}

func accountConfigFromSetup(opts *emailSetupOptions) (config.EmailAccountConfig, error) {
	acc := config.EmailAccountConfig{
		Name:    strings.TrimSpace(opts.Account),
		Backend: strings.ToLower(strings.TrimSpace(opts.Backend)),
	}
	switch acc.Backend {
	case "imap":
		if strings.TrimSpace(opts.IMAPHost) == "" || strings.TrimSpace(opts.IMAPUser) == "" || strings.TrimSpace(opts.IMAPPass) == "" {
			return config.EmailAccountConfig{}, fmt.Errorf("imap backend requires --imap-host, --imap-user, and --imap-pass")
		}
		if strings.TrimSpace(opts.SMTPHost) == "" || strings.TrimSpace(opts.SMTPUser) == "" || strings.TrimSpace(opts.SMTPPass) == "" {
			return config.EmailAccountConfig{}, fmt.Errorf("imap backend requires --smtp-host, --smtp-user, and --smtp-pass")
		}
		acc.IMAP = &config.EmailIMAPConfig{
			Host:     strings.TrimSpace(opts.IMAPHost),
			Username: strings.TrimSpace(opts.IMAPUser),
			Password: strings.TrimSpace(opts.IMAPPass),
			Mailbox:  "INBOX",
		}
		acc.SMTP = &config.EmailSMTPConfig{
			Host:     strings.TrimSpace(opts.SMTPHost),
			Port:     opts.SMTPPort,
			Username: strings.TrimSpace(opts.SMTPUser),
			Password: strings.TrimSpace(opts.SMTPPass),
			StartTLS: true,
		}
	case "gmail":
		acc.Gmail = &config.EmailGmailAPIConfig{
			TokenFile: filepath.ToSlash(filepath.Join("email", "oauth-"+acc.Name+"-gmail.json")),
		}
	case "graph":
		acc.Graph = &config.EmailGraphAPIConfig{
			TokenFile: filepath.ToSlash(filepath.Join("email", "oauth-"+acc.Name+"-graph.json")),
		}
	default:
		return config.EmailAccountConfig{}, fmt.Errorf("unsupported backend %q", acc.Backend)
	}
	return acc, nil
}

func upsertEmailAccount(accounts []config.EmailAccountConfig, acc config.EmailAccountConfig) []config.EmailAccountConfig {
	for i := range accounts {
		if strings.EqualFold(strings.TrimSpace(accounts[i].Name), strings.TrimSpace(acc.Name)) {
			accounts[i] = acc
			return accounts
		}
	}
	return append(accounts, acc)
}

func loadOrInitConfigForSetup(path string) (*config.Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		return &config.Config{
			Version:  1,
			StateDir: filepath.Dir(path),
		}, nil
	}
	var cfg config.Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if strings.TrimSpace(cfg.StateDir) == "" {
		cfg.StateDir = filepath.Dir(path)
	}
	if cfg.Version == 0 {
		cfg.Version = 1
	}
	return &cfg, nil
}

func writeConfigForSetup(path string, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("nil config")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	out, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o600)
}

func runEmailOAuthLoginGmail(gf *globalFlags, account string) error {
	cmd := emailLoginGmailCmd(gf)
	if err := cmd.Flags().Set("account", account); err != nil {
		return err
	}
	return cmd.RunE(cmd, nil)
}

func runEmailOAuthLoginGraph(gf *globalFlags, account string) error {
	cmd := emailLoginGraphCmd(gf)
	if err := cmd.Flags().Set("account", account); err != nil {
		return err
	}
	return cmd.RunE(cmd, nil)
}

func detectNotifySessionFromHistory(cfg *config.Config, channel string) (string, error) {
	if cfg == nil {
		return "", fmt.Errorf("config unavailable")
	}
	ch := strings.ToLower(strings.TrimSpace(channel))
	prefix := ""
	switch ch {
	case "telegram":
		prefix = "tg:"
	case "slack":
		prefix = "slack:"
	case "discord":
		prefix = "discord:"
	default:
		return "", fmt.Errorf("unsupported channel %q", channel)
	}
	dbPath := strings.TrimSpace(cfg.Memory.EpisodicDBPath)
	if dbPath == "" {
		dbPath = filepath.Join(cfg.StateDir, "memory", "episodic.db")
	}
	if _, err := os.Stat(dbPath); err != nil {
		return "", fmt.Errorf("episodic memory DB not found at %s; send one message from %s first", dbPath, ch)
	}
	ep, err := memory.NewEpisodicMemory(dbPath)
	if err != nil {
		return "", err
	}
	defer ep.Close()
	ids, err := ep.ListSessions(context.Background(), 200)
	if err != nil {
		return "", err
	}
	for _, id := range ids {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(id)), prefix) {
			return id, nil
		}
	}
	return "", fmt.Errorf("no recent %s sessions found in memory history", ch)
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
