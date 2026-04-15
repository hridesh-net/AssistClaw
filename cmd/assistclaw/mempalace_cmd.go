package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/mempalace"
	"github.com/spf13/cobra"
)

func mempalaceCmd(gf *globalFlags) *cobra.Command {
	var stateDir string
	cmd := &cobra.Command{
		Use:   "mempalace",
		Short: "Bootstrap or check the optional MemPalace (Python) integration",
	}
	cmd.PersistentFlags().StringVar(&stateDir, "state-dir", "",
		"AssistClaw state directory (e.g. ~/.assistclaw); uses env-style defaults without assistclaw.yaml — for install scripts")
	cmd.AddCommand(mempalaceSetupCmd(gf, &stateDir))
	cmd.AddCommand(mempalaceDoctorCmd(gf, &stateDir))
	return cmd
}

func mempalaceLoadCfg(gf *globalFlags, stateDir string) (*config.Config, error) {
	if strings.TrimSpace(stateDir) != "" {
		return config.MemPalaceBootstrapConfig(stateDir), nil
	}
	return loadConfig(gf.configPath, buildLogger(gf.logLevel))
}

func mempalaceSetupCmd(gf *globalFlags, stateDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "setup",
		Short: "Create managed venv, install mempalace from PyPI, and run mempalace init once",
		Long: `Runs the same steps as memory.mempalace.managed_venv on agent start:
  - Python venv under <state_dir>/mempalace/venv
  - pip install mempalace
  - mempalace init <state_dir>/mempalace/world

Requires a system python with the venv module (default: python3). Override with
ASSISTCLAW_MEMPALACE_BOOTSTRAP_PYTHON or memory.mempalace.bootstrap_python in config.

Use --state-dir from install.sh when assistclaw.yaml does not exist yet.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := context.Background()
			log := buildLogger(gf.logLevel)
			cfg, err := mempalaceLoadCfg(gf, *stateDir)
			if err != nil {
				return err
			}
			bp := strings.TrimSpace(cfg.Memory.MemPalace.BootstrapPython)
			if bp == "" {
				bp = strings.TrimSpace(os.Getenv("ASSISTCLAW_MEMPALACE_BOOTSTRAP_PYTHON"))
			}
			if bp == "" {
				bp = "python3"
			}
			fmt.Fprintln(os.Stderr, "mempalace setup: using state_dir", cfg.StateDir)
			if err := mempalace.Ensure(ctx, mempalace.EnsureOptions{
				StateDir:        cfg.StateDir,
				BootstrapPython: bp,
				Progress:        os.Stderr,
				Log:             log,
			}); err != nil {
				return err
			}
			fmt.Fprintln(os.Stderr, "mempalace setup: ok")
			fmt.Println("Add to assistclaw.yaml (or merge with your existing memory block):")
			fmt.Println("memory:")
			fmt.Println("  mempalace:")
			fmt.Println("    enabled: true")
			fmt.Println("    auto_start: true")
			fmt.Println("    managed_venv: true")
			fmt.Println("    inject_into_memory_search: false   # optional: true to merge into memory_search")
			return nil
		},
	}
}

func mempalaceDoctorCmd(gf *globalFlags, stateDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Report MemPalace managed paths and whether the venv can import mempalace",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := mempalaceLoadCfg(gf, *stateDir)
			if err != nil {
				return err
			}
			venv := mempalace.ManagedVenvRoot(cfg.StateDir)
			world := mempalace.ManagedWorldDir(cfg.StateDir)
			py := mempalace.VenvInterpreter(venv)
			fmt.Println("state_dir:     ", cfg.StateDir)
			fmt.Println("managed venv:  ", venv)
			fmt.Println("managed world: ", world)
			fmt.Println("venv python:   ", py)
			if _, err := os.Stat(py); err != nil {
				fmt.Println("import mempalace: (skip — venv python missing; run assistclaw mempalace setup)")
				return nil
			}
			c := exec.Command(py, "-c", "import mempalace; print('ok', mempalace.__file__)")
			c.Stdout = os.Stdout
			c.Stderr = os.Stderr
			if err := c.Run(); err != nil {
				fmt.Println("import mempalace: FAIL —", err)
				fmt.Println("hint: assistclaw mempalace setup")
				return nil
			}
			if _, err := os.Stat(mempalace.WorldInitMarker(cfg.StateDir)); err != nil {
				fmt.Println("world init marker: missing (run assistclaw mempalace setup)")
			} else {
				fmt.Println("world init marker: present")
			}
			return nil
		},
	}
}
