//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/spf13/cobra"
)

const systemdUnit = `[Unit]
Description=AssistClaw AI Agent
After=network.target

[Service]
Type=simple
ExecStart={{.BinaryPath}} start
Restart=always
RestartSec=5
StandardOutput=append:{{.LogDir}}/assistclaw.log
StandardError=append:{{.LogDir}}/assistclaw.log

[Install]
WantedBy=default.target
`

type unitData struct {
	BinaryPath string
	LogDir     string
}

func unitPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "assistclaw.service"), nil
}

func logDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, ".assistclaw", "logs")
	return dir, os.MkdirAll(dir, 0o755)
}

func installService() error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not resolve binary path: %w", err)
	}

	ld, err := logDir()
	if err != nil {
		return fmt.Errorf("could not create log dir: %w", err)
	}

	uPath, err := unitPath()
	if err != nil {
		return err
	}

	tmpl, err := template.New("unit").Parse(systemdUnit)
	if err != nil {
		return err
	}

	f, err := os.Create(uPath)
	if err != nil {
		return fmt.Errorf("could not write unit file: %w", err)
	}
	defer f.Close()
	if err := tmpl.Execute(f, unitData{BinaryPath: binaryPath, LogDir: ld}); err != nil {
		return err
	}
	f.Close()

	for _, args := range [][]string{
		{"--user", "daemon-reload"},
		{"--user", "enable", "--now", "assistclaw"},
	} {
		out, err := exec.Command("systemctl", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl %v failed: %s: %w", args, string(out), err)
		}
	}

	fmt.Printf("✅ AssistClaw installed as a systemd user service.\n")
	fmt.Printf("   Unit:  %s\n", uPath)
	fmt.Printf("   Logs:  %s/assistclaw.log\n", ld)
	fmt.Printf("   AssistClaw will start automatically on login.\n")
	return nil
}

func uninstallService() error {
	uPath, err := unitPath()
	if err != nil {
		return err
	}
	if _, err := os.Stat(uPath); os.IsNotExist(err) {
		return fmt.Errorf("service not installed (no unit at %s)", uPath)
	}

	for _, args := range [][]string{
		{"--user", "disable", "--now", "assistclaw"},
	} {
		out, err := exec.Command("systemctl", args...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("systemctl %v failed: %s: %w", args, string(out), err)
		}
	}

	if err := os.Remove(uPath); err != nil {
		return fmt.Errorf("could not remove unit file: %w", err)
	}

	fmt.Println("✅ AssistClaw systemd service removed.")
	return nil
}

func tailServiceLogs() error {
	ld, err := logDir()
	if err != nil {
		return err
	}
	logFile := filepath.Join(ld, "assistclaw.log")
	fmt.Printf("Tailing %s (Ctrl+C to stop)\n", logFile)
	cmd := exec.Command("tail", "-f", logFile)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func serviceInstallCmd(_ *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Register AssistClaw as a systemd user service (auto-start on login)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return installService()
		},
	}
}

func serviceUninstallCmd(_ *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the AssistClaw systemd user service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return uninstallService()
		},
	}
}

func serviceLogsCmd(_ *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "logs",
		Short: "Tail the AssistClaw daemon log file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tailServiceLogs()
		},
	}
}
