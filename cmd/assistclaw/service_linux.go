//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/spf13/cobra"
)

const systemdUnit = `[Unit]
Description=AssistClaw AI Agent
After=network.target

[Service]
Type=simple
{{- if .StateDir}}
Environment="ASSISTCLAW_STATE_DIR={{.StateDir}}"
{{- end}}
ExecStart={{.BinaryPath}} start
Restart=on-failure
RestartSec=5
StartLimitBurst=5
StartLimitIntervalSec=300
StandardOutput=append:{{.LogDir}}/assistclaw.log
StandardError=append:{{.LogDir}}/assistclaw.log
NoNewPrivileges=true
ProtectHome=read-only
ReadWritePaths={{.StateDir}}

[Install]
WantedBy=default.target
`

type unitData struct {
	BinaryPath string
	LogDir     string
	StateDir   string
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
	stateDir := strings.TrimSpace(os.Getenv("ASSISTCLAW_STATE_DIR"))
	if err := tmpl.Execute(f, unitData{BinaryPath: binaryPath, LogDir: ld, StateDir: stateDir}); err != nil {
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
	fmt.Printf("   Starts when your user systemd session runs (typically after graphical login).\n")
	printLinuxLingerHint()
	return nil
}

// printLinuxLingerHint explains systemd --user + reboot: without linger, the service may not
// run until someone logs in; with linger, the user manager starts at boot.
func printLinuxLingerHint() {
	u, err := user.Current()
	if err != nil {
		return
	}
	name := u.Username
	if name == "" {
		return
	}
	out, err := exec.Command("loginctl", "show-user", name, "-p", "Linger").CombinedOutput()
	if err != nil {
		return
	}
	line := strings.TrimSpace(string(out))
	if strings.Contains(line, "Linger=yes") {
		fmt.Printf("   ✓ loginctl linger is enabled — user services can start at boot without an active login.\n")
		return
	}
	fmt.Println()
	fmt.Println("   ━━ Headless / SSH server / want AssistClaw before anyone logs in? ━━")
	fmt.Println("   systemd user units often do not start at reboot until a session exists.")
	fmt.Printf("   Enable lingering for %q (one-time, requires sudo):\n", name)
	fmt.Printf("      sudo loginctl enable-linger %s\n", name)
	fmt.Println("   Then reboot, or run: systemctl --user start assistclaw")
	fmt.Println()
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

func serviceStatusCmd(_ *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show systemd user unit and loginctl linger state",
		RunE: func(cmd *cobra.Command, args []string) error {
			uPath, err := unitPath()
			if err != nil {
				return err
			}
			if _, err := os.Stat(uPath); os.IsNotExist(err) {
				fmt.Println("Service: not installed (no unit file).")
				fmt.Println("Run: assistclaw service install")
				return nil
			}
			out, err := exec.Command("systemctl", "--user", "is-active", "assistclaw").CombinedOutput()
			state := strings.TrimSpace(string(out))
			if err != nil && state == "" {
				state = "unknown (" + err.Error() + ")"
			}
			fmt.Printf("systemd user unit assistclaw: %s\n", state)
			out, _ = exec.Command("systemctl", "--user", "is-enabled", "assistclaw").CombinedOutput()
			fmt.Printf("enabled at login: %s\n", strings.TrimSpace(string(out)))

			if u, err := user.Current(); err == nil && u.Username != "" {
				lo, _ := exec.Command("loginctl", "show-user", u.Username, "-p", "Linger").CombinedOutput()
				fmt.Printf("loginctl linger (%s): %s\n", u.Username, strings.TrimSpace(string(lo)))
			}
			ld, _ := logDir()
			fmt.Printf("log file: %s\n", filepath.Join(ld, "assistclaw.log"))
			return nil
		},
	}
}
