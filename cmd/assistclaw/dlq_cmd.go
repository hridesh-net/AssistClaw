package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/spf13/cobra"
)

type dlqRecord struct {
	FailedAt       time.Time `json:"failed_at"`
	Channel        string    `json:"channel"`
	SessionID      string    `json:"session_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Text           string    `json:"text"`
	Reason         string    `json:"reason"`
	Attempts       int       `json:"attempts"`
}

func dlqCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dlq",
		Short: "Inspect outbound dead-letter queue",
	}
	cmd.AddCommand(dlqListCmd(gf))
	return cmd
}

func dlqListCmd(gf *globalFlags) *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "list",
		Short: "List recent DLQ records",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := gf.configPath
			if strings.TrimSpace(path) == "" {
				path = config.DefaultConfigPath()
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			dlqPath := cfg.Channels.Outbound.DLQPath
			if strings.TrimSpace(dlqPath) == "" {
				dlqPath = filepath.Join(cfg.StateDir, "channels", "dlq.ndjson")
			}
			f, err := os.Open(dlqPath)
			if err != nil {
				if os.IsNotExist(err) {
					fmt.Printf("DLQ is empty (file not found): %s\n", dlqPath)
					return nil
				}
				return err
			}
			defer f.Close()

			var records []dlqRecord
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				if line == "" {
					continue
				}
				var r dlqRecord
				if err := json.Unmarshal([]byte(line), &r); err == nil {
					records = append(records, r)
				}
			}
			if err := sc.Err(); err != nil {
				return err
			}
			if len(records) == 0 {
				fmt.Printf("DLQ is empty: %s\n", dlqPath)
				return nil
			}
			start := 0
			if limit > 0 && len(records) > limit {
				start = len(records) - limit
			}
			fmt.Printf("DLQ file: %s\n", dlqPath)
			for _, r := range records[start:] {
				fmt.Printf("- %s  channel=%s attempts=%d session=%s key=%s\n  reason=%s\n",
					r.FailedAt.Format(time.RFC3339), r.Channel, r.Attempts, r.SessionID, r.IdempotencyKey, r.Reason)
			}
			return nil
		},
	}
	c.Flags().IntVar(&limit, "limit", 50, "Maximum number of records to print")
	return c
}
