package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/assistclaw/assistclaw/internal/automation"
	"github.com/assistclaw/assistclaw/internal/gateway"
	"github.com/assistclaw/assistclaw/internal/voice"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// ─────────────────────────────────────────────
// gateway command (start · stop · restart · serve)
// ─────────────────────────────────────────────

func gatewayCmd(gf *globalFlags) *cobra.Command {
	var skipPreflight, preflightFull bool
	cmd := &cobra.Command{
		Use:   "gateway",
		Short: "Manage the AssistClaw Gateway and Web UI",
		Long: `Manage the AssistClaw background gateway and embedded web UI.

Subcommands:
  start    Start in background daemon mode (web UI + channels)
  stop     Stop the running background daemon
  restart  Restart the background daemon
  serve    Run the gateway in the foreground (blocks terminal)
  status   Show daemon status (alias of 'assistclaw status')

By default, start and serve run a fast preflight (doctor subset, --skip-network) before binding. Use --preflight-full for full network checks.`,
	}
	registerGatewayPreflightFlags(cmd, &skipPreflight, &preflightFull)

	// gateway start — alias of 'assistclaw start --daemon'
	cmd.AddCommand(&cobra.Command{
		Use:   "start",
		Short: "Start AssistClaw daemon in background (web UI + agent + channels)",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck
			pctx, cancel := context.WithTimeout(context.Background(), preflightDefaultTimeout)
			defer cancel()
			if err := runPreflight(pctx, gf, preflightOpts{Skip: skipPreflight, Full: preflightFull}, log, cmd.ErrOrStderr()); err != nil {
				return err
			}
			return Detach("start")
		},
	})

	// gateway stop — alias of 'assistclaw stop'
	cmd.AddCommand(&cobra.Command{
		Use:   "stop",
		Short: "Stop the running AssistClaw background daemon",
		RunE:  stopCmd(gf).RunE,
	})

	// gateway restart — alias of 'assistclaw restart'
	cmd.AddCommand(&cobra.Command{
		Use:   "restart",
		Short: "Restart the AssistClaw background daemon",
		RunE:  restartCmd(gf).RunE,
	})

	// gateway status — alias of 'assistclaw status'
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show AssistClaw daemon status, PID, and web UI address",
		RunE:  statusCmd(gf).RunE,
	})

	// gateway serve — foreground gateway-only server (dev/debug)
	cmd.AddCommand(&cobra.Command{
		Use:   "serve",
		Short: "Run the gateway in the foreground (blocks terminal)",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			defer log.Sync() //nolint:errcheck

			pctx, cancel := context.WithTimeout(context.Background(), preflightDefaultTimeout)
			defer cancel()
			if err := runPreflight(pctx, gf, preflightOpts{Skip: skipPreflight, Full: preflightFull}, log, cmd.ErrOrStderr()); err != nil {
				return err
			}

			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			log.Info("Starting AssistClaw Gateway (foreground)...",
				zap.String("host", cfg.Gateway.Host),
				zap.Int("port", cfg.Gateway.Port),
				zap.String("bind", cfg.Gateway.Bind),
			)
			srv := gateway.NewServer(cfg.Gateway.Port)
			srv.Bind = cfg.Gateway.Bind
			srv.Tailscale.Mode = cfg.Gateway.Tailscale.Mode
			srv.Token = cfg.Gateway.Token
			srv.Version = version
			srv.Config = cfg
			srv.Logger = log

			if cfg.Gmail.Enabled {
				srv.Gmail = automation.NewGmailWatcher(cfg.Gmail, log)
			}
			if cfg.Voice.Enabled {
				srv.Voice = voice.NewDaemon(cfg.Voice)
			}

			webHost := cfg.Gateway.Host
			if webHost == "" {
				webHost = "localhost"
			}
			fmt.Printf("\n🌐 Web UI: http://%s:%d\n", webHost, cfg.Gateway.Port)

			errCh := make(chan error, 1)
			go func() {
				if err := srv.Start(context.Background()); err != nil && err != http.ErrServerClosed {
					errCh <- err
				}
			}()

			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

			select {
			case err := <-errCh:
				return fmt.Errorf("gateway error: %w", err)
			case <-sigCh:
				log.Info("Shutting down Gateway...")
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				if err := srv.Stop(ctx); err != nil {
					return fmt.Errorf("shutdown error: %w", err)
				}
			}
			return nil
		},
	})

	return cmd
}
