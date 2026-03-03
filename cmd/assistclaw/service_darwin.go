//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/spf13/cobra"
)

const launchdPlist = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.assistclaw.agent</string>

    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>start</string>
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>{{.LogDir}}/assistclaw.log</string>

    <key>StandardErrorPath</key>
    <string>{{.LogDir}}/assistclaw.log</string>

    <key>ThrottleInterval</key>
    <integer>10</integer>
</dict>
</plist>
`

type plistData struct {
	BinaryPath string
	LogDir     string
}

func plistPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", "com.assistclaw.agent.plist"), nil
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

	pPath, err := plistPath()
	if err != nil {
		return err
	}

	// Render plist
	tmpl, err := template.New("plist").Parse(launchdPlist)
	if err != nil {
		return err
	}

	f, err := os.Create(pPath)
	if err != nil {
		return fmt.Errorf("could not write plist: %w", err)
	}
	defer f.Close()

	if err := tmpl.Execute(f, plistData{BinaryPath: binaryPath, LogDir: ld}); err != nil {
		return err
	}
	f.Close()

	// Load with launchctl
	out, err := exec.Command("launchctl", "load", "-w", pPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl load failed: %s: %w", string(out), err)
	}

	fmt.Printf("✅ AssistClaw installed as a launch agent.\n")
	fmt.Printf("   Plist: %s\n", pPath)
	fmt.Printf("   Logs:  %s/assistclaw.log\n", ld)
	fmt.Printf("   AssistClaw will start automatically on login.\n")
	return nil
}

func uninstallService() error {
	pPath, err := plistPath()
	if err != nil {
		return err
	}

	if _, err := os.Stat(pPath); os.IsNotExist(err) {
		return fmt.Errorf("service not installed (no plist at %s)", pPath)
	}

	out, err := exec.Command("launchctl", "unload", "-w", pPath).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl unload failed: %s: %w", string(out), err)
	}

	if err := os.Remove(pPath); err != nil {
		return fmt.Errorf("could not remove plist: %w", err)
	}

	fmt.Println("✅ AssistClaw launch agent removed.")
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
		Short: "Register AssistClaw as a macOS launch agent (auto-start on login)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return installService()
		},
	}
}

func serviceUninstallCmd(_ *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the AssistClaw macOS launch agent",
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
