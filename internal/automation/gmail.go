package automation

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/config"
)

// GmailWatcher manages the gogcli daemon for Gmail Pub/Sub watching.
type GmailWatcher struct {
	cfg    config.GmailConfig
	log    *zap.Logger
	cancel context.CancelFunc
}

func NewGmailWatcher(cfg config.GmailConfig, logger *zap.Logger) *GmailWatcher {
	return &GmailWatcher{
		cfg: cfg,
		log: logger,
	}
}

// Start launches the gogcli watch serve daemon if enabled.
func (gw *GmailWatcher) Start(ctx context.Context) error {
	if !gw.cfg.Enabled || gw.cfg.SkipWatcher {
		return nil
	}

	if gw.cfg.Account == "" {
		return fmt.Errorf("gmail: account not configured")
	}

	runCtx, cancel := context.WithCancel(ctx)
	gw.cancel = cancel

	go gw.run(runCtx)

	return nil
}

func (gw *GmailWatcher) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			gw.log.Info("gmail: starting gogcli watch serve", zap.String("account", gw.cfg.Account))

			// gog gmail watch serve --account ... --hook-url ...
			args := []string{
				"gmail", "watch", "serve",
				"--account", gw.cfg.Account,
				"--include-body",
				"--max-bytes", "10000",
			}

			// In a real implementation, we'd point this to our /api/webhook/gmail endpoint
			// For now, this is a placeholder for the external process management logic
			cmd := exec.CommandContext(ctx, "gog", args...)
			
			if err := cmd.Run(); err != nil {
				if ctx.Err() != nil {
					return
				}
				gw.log.Error("gmail: gogcli exited with error", zap.Error(err))
				// Wait before restarting
				time.Sleep(10 * time.Second)
			}
		}
	}
}

func (gw *GmailWatcher) Stop() {
	if gw.cancel != nil {
		gw.cancel()
	}
}
