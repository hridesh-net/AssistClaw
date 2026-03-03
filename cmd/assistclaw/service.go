// Package main — service command: register AssistClaw with the OS init system.
package main

import (
	"github.com/spf13/cobra"
)

// serviceCmd returns the `assistclaw service` command group.
func serviceCmd(gf *globalFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "service",
		Short: "Manage the AssistClaw background service (auto-start on login)",
	}
	cmd.AddCommand(serviceInstallCmd(gf))
	cmd.AddCommand(serviceUninstallCmd(gf))
	cmd.AddCommand(serviceLogsCmd(gf))
	return cmd
}
