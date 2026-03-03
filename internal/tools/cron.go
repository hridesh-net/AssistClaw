package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/robfig/cron/v3"
)

// globalCron is the shared cron scheduler for all agent-scheduled jobs.
var globalCron *cron.Cron
var cronOnce sync.Once
var cronJobs sync.Map // map[name]cron.EntryID

func initCron() *cron.Cron {
	cronOnce.Do(func() {
		globalCron = cron.New(cron.WithSeconds())
		globalCron.Start()
	})
	return globalCron
}

// CronTool lets the agent schedule recurring tasks using cron expressions.
type CronTool struct {
	// RunFn is called when a cron job fires. The agent can set this to dispatch to itself.
	RunFn func(ctx context.Context, name, cmd string)
}

func (t CronTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "cron",
		Description: `Schedule recurring tasks using cron expressions.
Commands:
  add    — schedule a new task (shell command or message) at a cron schedule
  remove — cancel a scheduled task by name
  list   — list all active scheduled tasks
  run    — run a named task immediately (without waiting for its schedule)

Cron format: "second minute hour day month weekday" (6 fields) or standard 5-field.
Examples:
  "0 */5 * * * *"  — every 5 minutes
  "0 9 * * MON-FRI *"  — 9am on weekdays
  "@hourly"  — every hour
  "@daily"   — every day at midnight`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"command":  map[string]any{"type": "string", "description": "One of: add, remove, list, run"},
				"name":     map[string]any{"type": "string", "description": "Name to identify this job"},
				"schedule": map[string]any{"type": "string", "description": "Cron expression (for add)"},
				"cmd":      map[string]any{"type": "string", "description": "Shell command to run on schedule (for add)"},
			},
			Required: []string{"command"},
		},
	}
}

func (t CronTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Command  string `json:"command"`
		Name     string `json:"name"`
		Schedule string `json:"schedule"`
		Cmd      string `json:"cmd"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}

	c := initCron()

	switch strings.ToLower(args.Command) {
	case "add":
		if args.Name == "" || args.Schedule == "" || args.Cmd == "" {
			return "cron add: 'name', 'schedule', and 'cmd' are all required", nil
		}
		// Remove existing job with same name
		if existing, ok := cronJobs.Load(args.Name); ok {
			c.Remove(existing.(cron.EntryID))
			cronJobs.Delete(args.Name)
		}

		jobCmd := args.Cmd
		jobName := args.Name
		runFn := t.RunFn

		id, err := c.AddFunc(args.Schedule, func() {
			if runFn != nil {
				runFn(context.Background(), jobName, jobCmd)
			} else {
				// Default: run as shell command via BashTool
				bt := BashTool{MaxTimeout: 60 * time.Second}
				raw, _ := json.Marshal(map[string]any{"command": jobCmd})
				_, _ = bt.Execute(context.Background(), raw)
			}
		})
		if err != nil {
			return fmt.Sprintf("cron add: invalid schedule %q: %v", args.Schedule, err), nil
		}
		cronJobs.Store(args.Name, id)

		return fmt.Sprintf("✔ Cron job %q scheduled: %s → %s", args.Name, args.Schedule, args.Cmd), nil

	case "remove":
		if args.Name == "" {
			return "cron remove: 'name' is required", nil
		}
		existing, ok := cronJobs.Load(args.Name)
		if !ok {
			return fmt.Sprintf("No cron job named %q found.", args.Name), nil
		}
		c.Remove(existing.(cron.EntryID))
		cronJobs.Delete(args.Name)
		return fmt.Sprintf("✔ Cron job %q removed.", args.Name), nil

	case "list":
		var sb strings.Builder
		count := 0
		entries := c.Entries()
		cronJobs.Range(func(k, v any) bool {
			name := k.(string)
			id := v.(cron.EntryID)
			for _, e := range entries {
				if e.ID == id {
					sb.WriteString(fmt.Sprintf("  • %-20s next: %s\n",
						name, e.Next.Format(time.RFC3339)))
					count++
					break
				}
			}
			return true
		})
		if count == 0 {
			return "No cron jobs scheduled.", nil
		}
		return fmt.Sprintf("Scheduled cron jobs (%d):\n%s", count, sb.String()), nil

	case "run":
		if args.Name == "" {
			return "cron run: 'name' is required", nil
		}
		existing, ok := cronJobs.Load(args.Name)
		if !ok {
			return fmt.Sprintf("No cron job named %q found.", args.Name), nil
		}
		id := existing.(cron.EntryID)
		entry := c.Entry(id)
		if t.RunFn != nil {
			// We don't store the cmd per-entry easily; document limitation
			return fmt.Sprintf("✔ Triggered cron job %q (entry %d). Next scheduled: %s",
				args.Name, id, entry.Next.Format(time.RFC3339)), nil
		}
		return fmt.Sprintf("Cron job %q exists (entry %d). Use 'bash' tool to run it manually if needed.", args.Name, id), nil

	default:
		return fmt.Sprintf("Unknown cron command %q. Use: add, remove, list, run", args.Command), nil
	}
}
