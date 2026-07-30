package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/assistclaw/assistclaw/internal/embeddings"
	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/spf13/cobra"
)

// ─────────────────────────────────────────────
// memory command
// ─────────────────────────────────────────────

func memoryCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{Use: "memory", Short: "Search and manage conversation memory"}
	cmd.AddCommand(&cobra.Command{
		Use:   "search [query]",
		Short: "Full-text search of conversation history",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			epMem, err := memory.NewEpisodicMemory(cfg.Memory.EpisodicDBPath)
			if err != nil {
				return err
			}
			defer epMem.Close()
			results, err := epMem.Search(ctx, args[0], 20)
			if err != nil {
				return err
			}
			for _, m := range results {
				fmt.Printf("[%s] %s: %s\n\n", m.CreatedAt.Format("2006-01-02 15:04"), m.Role, m.Content)
			}
			return nil
		},
	})
	mineCmd := &cobra.Command{Use: "mine", Short: "Mine/backfill taxonomy metadata for memory files"}
	var mineJSON bool
	var mineDryRun bool
	var mineMode string
	var mineLimit int
	var mineYes bool
	mineCmd.PersistentFlags().BoolVar(&mineJSON, "json", false, "Output JSON")
	mineCmd.PersistentFlags().BoolVar(&mineDryRun, "dry-run", false, "Plan run without indexing writes")
	mineCmd.PersistentFlags().StringVar(&mineMode, "mode", "", "Mining mode override: incremental|full")
	mineCmd.PersistentFlags().IntVar(&mineLimit, "limit", 0, "Maximum files to process")

	runMine := func(forceMode string) (*memory.MiningReport, error) {
		ctx := context.Background()
		log := buildLogger(gf.logLevel)
		cfg, err := loadConfig(gf.configPath, log)
		if err != nil {
			return nil, err
		}
		embedReg := embeddings.NewRegistry()
		registerEmbedders(ctx, cfg, embedReg, log)
		memMgr, err := memory.NewManager(memory.ManagerConfig{
			WorkingTokenBudget:  cfg.Memory.WorkingTokenBudget,
			EpisodicDBPath:      cfg.Memory.EpisodicDBPath,
			SemanticDBPath:      cfg.Memory.SemanticDBPath,
			EmbeddingDimensions: 1536,
			ChunkSize:           cfg.Memory.Mining.ChunkSize,
			ChunkOverlap:        cfg.Memory.Mining.ChunkOverlap,
		})
		if err != nil {
			return nil, err
		}
		defer memMgr.Close()
		mode := cfg.Memory.Mining.Mode
		if mineMode != "" {
			mode = mineMode
		}
		if forceMode != "" {
			mode = forceMode
		}
		maxFiles := cfg.Memory.Mining.MaxFilesPerRun
		if mineLimit > 0 {
			maxFiles = mineLimit
		}
		report, err := memMgr.Mine(ctx, embedReg, cfg.StateDir, memory.MiningOptions{
			Mode:           mode,
			Include:        cfg.Memory.Mining.Include,
			Exclude:        cfg.Memory.Mining.Exclude,
			MaxFilesPerRun: maxFiles,
			MaxFileSizeKB:  cfg.Memory.Mining.MaxFileSizeKB,
			StatePath:      cfg.Memory.Mining.StatePath,
			DryRun:         mineDryRun,
		})
		if err != nil {
			return nil, err
		}
		return &report, nil
	}

	mineCmd.AddCommand(&cobra.Command{
		Use:   "run",
		Short: "Run an incremental mining pass",
		RunE: func(cmd *cobra.Command, args []string) error {
			report, err := runMine("")
			if err != nil {
				return err
			}
			if mineJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("memory mine run complete: %s\n", report.PrettyString())
			return nil
		},
	})
	mineCmd.AddCommand(&cobra.Command{
		Use:   "backfill",
		Short: "Run a full backfill over all included files",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !mineYes {
				return fmt.Errorf("backfill requires --yes")
			}
			report, err := runMine("full")
			if err != nil {
				return err
			}
			if mineJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("memory mine backfill complete: %s\n", report.PrettyString())
			return nil
		},
	})
	mineCmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show last mining run status",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			report, err := memory.ReadMiningState(cfg.Memory.Mining.StatePath)
			if err != nil {
				return err
			}
			if mineJSON {
				data, _ := json.MarshalIndent(report, "", "  ")
				fmt.Println(string(data))
				return nil
			}
			fmt.Printf("memory mine status: %s\n", report.PrettyString())
			return nil
		},
	})
	mineCmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: "Validate mining config and embedder readiness",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}
			embedReg := embeddings.NewRegistry()
			registerEmbedders(context.Background(), cfg, embedReg, log)
			if _, ok := embedReg.Default(); !ok {
				return fmt.Errorf("no embedding provider available for mining")
			}
			if mineJSON {
				fmt.Println(`{"schema_version":1,"valid":true}`)
				return nil
			}
			fmt.Println("memory mine validate: ok")
			return nil
		},
	})
	mineCmd.PersistentFlags().BoolVar(&mineYes, "yes", false, "Confirm destructive/full backfill operations")
	cmd.AddCommand(mineCmd)
	return cmd
}
