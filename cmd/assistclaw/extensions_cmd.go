package main

import (
	"fmt"
	"strings"

	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	"github.com/spf13/cobra"
)

func extensionsCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extensions",
		Short: "Extension hooks and built-in equivalents (AssistClaw)",
		Long: `AssistClaw does not load third-party Node extension bundles. Built-in coverage:

  • Channels, skills, MCP clients, webhooks, cron, browser tools, voice — see list.
  • Optional prompt_files in assistclaw.yaml merge markdown into the system prompt
    (workspace prompt fragments as plain markdown, not executable plugins).

Extend behavior via skills, MCP, channels, or prompt_files as needed.`,
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "Summarize extension-equivalent features and extensions.* config",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			prim := func(s string) string { return tui.Style(tui.ColorPrimary, true, s) }
			dim := func(s string) string { return tui.Style(tui.ColorMuted, false, s) }

			fmt.Println(prim("Channels (in-process, not npm plugins)"))
			var ch []string
			if cfg.Channels.WhatsApp != nil {
				ch = append(ch, "whatsapp")
			}
			if cfg.Channels.Telegram != nil {
				ch = append(ch, "telegram")
			}
			if cfg.Channels.Discord != nil {
				ch = append(ch, "discord")
			}
			if cfg.Channels.Slack != nil {
				ch = append(ch, "slack")
			}
			if len(ch) == 0 {
				fmt.Println(dim("  (none configured)"))
			} else {
				fmt.Println(dim("  " + strings.Join(ch, ", ")))
			}

			fmt.Println(prim("\nMCP"))
			fmt.Printf("  server enabled: %v  transport: %s\n", cfg.MCP.Server.Enabled, cfg.MCP.Server.Transport)
			fmt.Printf("  external clients: %d\n", len(cfg.MCP.Clients))

			fmt.Println(prim("\nWebhooks"))
			if cfg.Webhooks.Enabled {
				fmt.Printf("  enabled, mappings: %d\n", len(cfg.Webhooks.Mappings))
			} else {
				fmt.Println(dim("  disabled"))
			}

			fmt.Println(prim("\nCron"))
			fmt.Printf("  jobs: %d\n", len(cfg.Cron))

			fmt.Println(prim("\nSkills"))
			fmt.Printf("  enabled in config: %d\n", len(cfg.Agent.EnabledSkills))

			fmt.Println(prim("\nVoice / browser / memory"))
			fmt.Println(dim("  voice: STT/TTS via internal/voice (yaml voice:)"))
			fmt.Println(dim("  browser: browser_navigate, browser_screenshot (chromedp)"))
			fmt.Println(dim("  memory: working + episodic.db + semantic (embeddings); optional MemPalace via mcp / memory.mempalace (managed_venv automates pip + init)"))

			fmt.Println(prim("\nextensions.prompt_files (extra markdown prompt fragments)"))
			if !cfg.Extensions.Enabled {
				fmt.Println(dim("  disabled (set extensions.enabled: true)"))
			} else if len(cfg.Extensions.PromptFiles) == 0 {
				fmt.Println(dim("  enabled, no files listed"))
			} else {
				for _, p := range cfg.Extensions.PromptFiles {
					fmt.Printf("  • %s\n", p)
				}
			}
			fmt.Println()
			return nil
		},
	})
	return cmd
}
