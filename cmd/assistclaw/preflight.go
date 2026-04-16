package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"go.uber.org/zap"

	"github.com/assistclaw/assistclaw/internal/observability/metrics"
)

const preflightDefaultTimeout = 120 * time.Second

type preflightOpts struct {
	Skip bool
	Full bool // run doctor with network (LLM + channel APIs); default is local-only (same as doctor --skip-network)
}

func registerPreflightFlags(cmd *cobra.Command, skipPreflight, preflightFull *bool) {
	cmd.Flags().BoolVar(skipPreflight, "skip-preflight", false, "Skip doctor preflight before starting (experts only; invalid config may still fail later)")
	cmd.Flags().BoolVar(preflightFull, "preflight-full", false, "Run full doctor checks including network APIs; default is fast local validation only")
}

func registerGatewayPreflightFlags(cmd *cobra.Command, skipPreflight, preflightFull *bool) {
	cmd.PersistentFlags().BoolVar(skipPreflight, "skip-preflight", false, "Skip doctor preflight before starting (experts only; invalid config may still fail later)")
	cmd.PersistentFlags().BoolVar(preflightFull, "preflight-full", false, "Run full doctor checks including network APIs; default is fast local validation only")
}

// runPreflight runs the same checks as assistclaw doctor (subset: same pipeline). Returns non-nil on doctor exit code 2.
func runPreflight(ctx context.Context, gf *globalFlags, opts preflightOpts, log *zap.Logger, stderr io.Writer) error {
	if opts.Skip {
		fmt.Fprintln(stderr, "WARNING: preflight skipped (--skip-preflight). Startup may fail with invalid config or unreachable APIs; only use when you understand the risk.")
		if log != nil {
			log.Warn("preflight skipped by user")
		}
		return nil
	}

	skipNetwork := !opts.Full
	channelTimeout := 15 * time.Second

	cfg, usedPath, cfgIssues, err := loadConfigForDoctor(gf.configPath)
	if err != nil {
		metrics.Default().IncPreflightFailures()
		return fmt.Errorf("preflight: config: %w", err)
	}
	_ = usedPath

	checks := doctorChecksFromConfigIssues(usedPath, cfgIssues)
	checks = append(checks, runDoctorChecks(ctx, cfg, skipNetwork, gf.logLevel, channelTimeout)...)

	exit := doctorExitCode(checks)
	if exit == 1 {
		for _, c := range checks {
			if c.Severity == "warn" {
				fmt.Fprintf(stderr, "preflight warning: [%s] %s: %s\n", c.Severity, c.ID, c.Message)
			}
		}
	}
	if exit >= 2 {
		metrics.Default().IncPreflightFailures()
		var b strings.Builder
		for _, c := range checks {
			if c.Severity == "error" {
				fmt.Fprintf(&b, "\n  [%s] %s: %s", c.Severity, c.ID, c.Message)
			}
		}
		return fmt.Errorf("preflight failed (fix with `assistclaw doctor` or use --skip-preflight only if you accept the risk):%s", b.String())
	}
	return nil
}

// execStart runs doctor preflight then starts the daemon (foreground or detached).
func execStart(gf *globalFlags, daemon bool, skipPreflight, preflightFull bool, cmd *cobra.Command) error {
	log := buildLogger(gf.logLevel)
	defer log.Sync() //nolint:errcheck

	pctx, cancel := context.WithTimeout(context.Background(), preflightDefaultTimeout)
	defer cancel()
	if err := runPreflight(pctx, gf, preflightOpts{Skip: skipPreflight, Full: preflightFull}, log, cmd.ErrOrStderr()); err != nil {
		return err
	}

	if daemon {
		return Detach("start")
	}
	return runAgent(gf, gf.configPath, "", "", "", true, false, false)
}
