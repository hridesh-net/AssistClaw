package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/tabwriter"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/assistclaw/assistclaw/internal/cron"
	"github.com/assistclaw/assistclaw/cmd/assistclaw/tui"
	"github.com/charmbracelet/lipgloss"
)

func cronCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cron",
		Short: "Manage persistent scheduled background jobs",
		Long: `Manage scheduled tasks that run in the background.

Jobs defined here are stored in ~/.assistclaw/cron_jobs.json and are
picked up by the background daemon (assistclaw start or assistclaw gateway start).`,
	}

	cmd.AddCommand(cronListCmd(gf))
	cmd.AddCommand(cronAddCmd(gf))
	cmd.AddCommand(cronRemoveCmd(gf))
	
	return cmd
}

func readCronJobs(path string) ([]cron.Job, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // Return empty list if it doesn't exist
		}
		return nil, err
	}
	var jobs []cron.Job
	if err := json.Unmarshal(data, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func writeCronJobs(path string, jobs []cron.Job) error {
	data, err := json.MarshalIndent(jobs, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func cronListCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all scheduled cron jobs",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			path := filepath.Join(cfg.StateDir, "cron_jobs.json")
			jobs, err := readCronJobs(path)
			if err != nil {
				return fmt.Errorf("failed to read cron jobs: %w", err)
			}

			prim := lipgloss.NewStyle().Foreground(tui.ColorPrimary).Bold(true)
			dim := lipgloss.NewStyle().Foreground(tui.ColorMuted)
			header := lipgloss.NewStyle().Foreground(tui.ColorNeon).Bold(true)

			fmt.Println(header.Render("\n🕒 Persistent Cron Jobs") + dim.Render(fmt.Sprintf("  (%d jobs)", len(jobs))))
			fmt.Println(dim.Render("─────────────────────────────────────────────────────────────"))
			
			if len(jobs) == 0 {
				fmt.Println(dim.Render("  No persistent jobs scheduled."))
				fmt.Println(dim.Render("  Run: assistclaw cron add \"@hourly\" \"Review the system logs\""))
			} else {
				w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintf(w, "  %s\t%s\t%s\n", dim.Render("ID"), dim.Render("SCHEDULE"), dim.Render("PROMPT"))
				for _, j := range jobs {
					fmt.Fprintf(w, "  %s\t%s\t%s\n", prim.Render(j.ID), j.Schedule, j.Prompt)
				}
				w.Flush()
			}
			
			if len(cfg.Cron) > 0 {
				fmt.Println(header.Render("\n⚙️  Static Jobs") + dim.Render(fmt.Sprintf("  (%d jobs from assistclaw.yaml)", len(cfg.Cron))))
				fmt.Println(dim.Render("─────────────────────────────────────────────────────────────"))
				w2 := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
				fmt.Fprintf(w2, "  %s\t%s\t%s\n", dim.Render("ID"), dim.Render("SCHEDULE"), dim.Render("PROMPT"))
				for _, j := range cfg.Cron {
					fmt.Fprintf(w2, "  %s\t%s\t%s\n", prim.Render(j.ID), j.Schedule, j.Prompt)
				}
				w2.Flush()
			}

			fmt.Println()
			return nil
		},
	}
}

func cronAddCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add [schedule] [prompt]",
		Short: "Add a new cron job",
		Example: `  assistclaw cron add "@hourly" "Check the system for errors"
  assistclaw cron add "0 9 * * *" "Generate daily report"`,
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			scheduleStr := args[0]
			promptStr := args[1]

			path := filepath.Join(cfg.StateDir, "cron_jobs.json")
			jobs, err := readCronJobs(path)
			if err != nil {
				return fmt.Errorf("failed to read cron jobs: %w", err)
			}

			newJob := cron.Job{
				ID:       uuid.New().String()[:8],
				Schedule: scheduleStr,
				Prompt:   promptStr,
			}
			jobs = append(jobs, newJob)

			if err := writeCronJobs(path, jobs); err != nil {
				return fmt.Errorf("failed to write cron jobs: %w", err)
			}

			fmt.Printf("✅ Job added with ID: %s\n", newJob.ID)
			return nil
		},
	}
	return cmd
}

func cronRemoveCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "remove [id]",
		Short: "Remove a cron job by ID",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			targetID := args[0]
			path := filepath.Join(cfg.StateDir, "cron_jobs.json")
			jobs, err := readCronJobs(path)
			if err != nil {
				return fmt.Errorf("failed to read cron jobs: %w", err)
			}

			filtered := make([]cron.Job, 0, len(jobs))
			removed := false
			for _, j := range jobs {
				if j.ID == targetID {
					removed = true
					continue
				}
				filtered = append(filtered, j)
			}

			if !removed {
				return fmt.Errorf("job ID %q not found", targetID)
			}

			if err := writeCronJobs(path, filtered); err != nil {
				return fmt.Errorf("failed to write cron jobs: %w", err)
			}

			fmt.Printf("🗑️  Job %q removed successfully.\n", targetID)
			return nil
		},
	}
	return cmd
}
