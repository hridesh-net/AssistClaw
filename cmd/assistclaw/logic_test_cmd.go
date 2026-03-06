package main

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/assistclaw/assistclaw/internal/skills"
	"github.com/spf13/cobra"
)

func logicTestCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logic-test",
		Short: "Internal logic verification suite",
	}

	cmd.AddCommand(graphBridgingTestCmd(gf))
	return cmd
}

func graphBridgingTestCmd(gf *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "graph-bridging",
		Short: "Verify skill graph bridging and wikilink extraction",
		RunE: func(cmd *cobra.Command, args []string) error {
			log := buildLogger(gf.logLevel)
			cfg, err := loadConfig(gf.configPath, log)
			if err != nil {
				return err
			}

			customDir := filepath.Join(cfg.StateDir, "skills", "custom")
			reg := skills.NewRegistry()
			if err := reg.LoadAll(context.Background(), customDir); err != nil {
				return fmt.Errorf("failed to load skills: %w", err)
			}

			fmt.Println("🚀 Running Graph Bridging Logic Test...")

			// 1. Verify link extraction
			allSkills := reg.List()
			if len(allSkills) == 0 {
				fmt.Println("ℹ No skills found to test.")
				return nil
			}

			linksFound := 0
			for _, s := range allSkills {
				for _, n := range s.Nodes {
					linksFound += len(n.Links)
				}
			}

			if linksFound > 0 {
				fmt.Printf("✔ Successfully extracted %d wikilinks across %d skills.\n", linksFound, len(allSkills))
			} else {
				fmt.Println("⚠ No wikilinks found. Ensure your skill nodes contain [[node-name]] syntax.")
			}

			// 2. Verify bridge discovery (if we have a skill with at least 2 nodes)
			for _, s := range allSkills {
				if len(s.Nodes) >= 2 {
					var nodePaths []string
					count := 0
					for _, n := range s.Nodes {
						nodePaths = append(nodePaths, n.FilePath)
						count++
						if count >= 2 {
							break
						}
					}

					bridges := reg.FindBridges(nodePaths)
					fmt.Printf("✔ Bridge discovery tested on skill %q: found %d bridges.\n", s.Name, len(bridges))
					break
				}
			}

			fmt.Println("✨ Graph Bridging Logic Test Completed.")
			return nil
		},
	}
}
