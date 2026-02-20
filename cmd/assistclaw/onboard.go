package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/spf13/cobra"
)

func onboardCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "onboard",
		Short: "Run the interactive first-time setup wizard",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := gf.configPath
			if path == "" {
				path = config.DefaultConfigPath()
			}
			return runOnboarding(path)
		},
	}
}

func runOnboarding(configPath string) error {
	fmt.Println("\n=======================================================")
	fmt.Println("             Welcome to AssistClaw! 🐾")
	fmt.Println("=======================================================")
	fmt.Println("Let's get you set up with your preferred AI provider.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	fmt.Println("Which primary provider would you like to use?")
	fmt.Println("  1) Anthropic (Recommended)")
	fmt.Println("  2) OpenAI")
	fmt.Println("  3) Ollama (Local/Free)")
	fmt.Print("\nEnter choice [1-3]: ")

	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)

	var tpl string
	switch choiceStr {
	case "2":
		fmt.Print("\nEnter your OpenAI API Key: ")
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  openai:
    api_key: "%s"
    default_model: "gpt-4o-mini"

routing:
  default: "openai/gpt-4o-mini"
`, key)

	case "3":
		fmt.Println("\nGreat! AssistClaw will look for Ollama at http://localhost:11434.")
		tpl = `# AssistClaw Configuration
version: 1

providers:
  ollama:
    base_url: "http://127.0.0.1:11434"
    default_model: "llama3.2"

routing:
  default: "ollama/llama3.2"
`

	case "1":
		fallthrough
	default:
		fmt.Print("\nEnter your Anthropic API Key: ")
		key, _ := reader.ReadString('\n')
		key = strings.TrimSpace(key)
		tpl = fmt.Sprintf(`# AssistClaw Configuration
version: 1

providers:
  anthropic:
    api_key: "%s"
    default_model: "claude-3-5-haiku-20241022"

routing:
  default: "anthropic/claude-3-5-haiku-20241022"
`, key)
	}

	// Make sure the directories exist.
	if err := config.InitializeWorkspace(configPath); err != nil {
		return err
	}

	// Write the configuration.
	if err := os.WriteFile(configPath, []byte(tpl), 0o600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	fmt.Printf("\nPerfect! Your configuration has been saved to %s\n", configPath)
	fmt.Println("You can edit this file at any time to add more providers or change router defaults.")
	fmt.Println("\nTry running your first command:")
	fmt.Println("  assistclaw agent -m \"Say hello!\"")
	fmt.Println("=======================================================")

	return nil
}
