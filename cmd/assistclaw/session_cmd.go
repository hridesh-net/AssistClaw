package main

import "github.com/spf13/cobra"

// ─────────────────────────────────────────────
// agent command
// ─────────────────────────────────────────────

func autoCmd(gf *globalFlags) *cobra.Command {
	var (
		model     string
		sessionID string
	)

	cmd := &cobra.Command{
		Use:   "auto [goal]",
		Short: "Start a continuous autonomous agent loop targeting a specific goal",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(gf, gf.configPath, model, args[0], sessionID, false, false, true)
		},
	}

	cmd.Flags().StringVar(&model, "model", "", "Model to use (e.g. anthropic/claude-3-5-sonnet)")
	cmd.Flags().StringVar(&sessionID, "session", "", "Resume an existing session by ID")
	return cmd
}

func agentCmd(gf *globalFlags) *cobra.Command {
	var (
		message   string
		model     string
		noStream  bool
		sessionID string
		serve     bool
	)

	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Start an interactive agent session or send a single message",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgent(gf, gf.configPath, model, message, sessionID, serve, noStream, false)
		},
	}

	cmd.Flags().StringVarP(&message, "message", "m", "", "Single message to send (non-interactive)")
	cmd.Flags().StringVar(&model, "model", "", "Model to use (e.g. anthropic/claude-haiku-3-5)")
	cmd.Flags().BoolVar(&noStream, "no-stream", false, "Disable streaming output")
	cmd.Flags().StringVar(&sessionID, "session", "", "Resume an existing session by ID")
	cmd.Flags().BoolVarP(&serve, "serve", "s", false, "Run in background mode with Gateway and messaging channels active")
	return cmd
}
