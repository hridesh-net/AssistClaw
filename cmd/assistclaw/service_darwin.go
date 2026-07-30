//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
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
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>

    <key>StandardOutPath</key>
    <string>{{.LogDir}}/assistclaw.log</string>

    <key>StandardErrorPath</key>
    <string>{{.LogDir}}/assistclaw.log</string>

    <key>ThrottleInterval</key>
    <integer>5</integer>
{{- if .StateDir}}
    <key>EnvironmentVariables</key>
    <dict>
        <key>ASSISTCLAW_STATE_DIR</key>
        <string>{{.StateDir}}</string>
    </dict>
{{- end}}
</dict>
</plist>
`

type plistData struct {
	BinaryPath string
	LogDir     string
	StateDir   string // optional; passed to assistclaw start via ASSISTCLAW_STATE_DIR
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

	stateDir := strings.TrimSpace(os.Getenv("ASSISTCLAW_STATE_DIR"))
	if err := tmpl.Execute(f, plistData{BinaryPath: binaryPath, LogDir: ld, StateDir: stateDir}); err != nil {
		return err
	}
	f.Close()

	if err := launchctlBootstrap(pPath); err != nil {
		return err
	}

	fmt.Printf("✅ AssistClaw installed as a launch agent.\n")
	fmt.Printf("   Plist: %s\n", pPath)
	fmt.Printf("   Logs:  %s/assistclaw.log\n", ld)
	fmt.Printf("   AssistClaw starts when you log in to this Mac (LaunchAgent).\n")
	fmt.Println("   If it does not start after reboot, run: assistclaw service status")
	return nil
}

// launchctlBootstrap registers the agent. Prefer modern `bootstrap` (macOS 10.11+); fall back to `load -w`.
func launchctlBootstrap(pPath string) error {
	uid := strconv.Itoa(os.Getuid())
	domain := "gui/" + uid
	// Idempotent: ignore bootout errors (not loaded yet or older OS).
	_, _ = exec.Command("launchctl", "bootout", domain, pPath).CombinedOutput()
	out, err := exec.Command("launchctl", "bootstrap", domain, pPath).CombinedOutput()
	if err == nil {
		return nil
	}
	legacyOut, legacyErr := exec.Command("launchctl", "load", "-w", pPath).CombinedOutput()
	if legacyErr != nil {
		return fmt.Errorf("launchctl bootstrap failed: %s; load -w failed: %s: %w", string(out), string(legacyOut), legacyErr)
	}
	return nil
}

func launchctlRemove(pPath string) error {
	uid := strconv.Itoa(os.Getuid())
	domain := "gui/" + uid
	out, err := exec.Command("launchctl", "bootout", domain, pPath).CombinedOutput()
	if err == nil {
		return nil
	}
	legacyOut, legacyErr := exec.Command("launchctl", "unload", "-w", pPath).CombinedOutput()
	if legacyErr != nil {
		return fmt.Errorf("launchctl bootout failed: %s; unload -w failed: %s: %w", string(out), string(legacyOut), legacyErr)
	}
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

	if err := launchctlRemove(pPath); err != nil {
		return err
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

func serviceStatusCmd(_ *globalFlags) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show whether the LaunchAgent plist exists and is loaded",
		RunE: func(cmd *cobra.Command, args []string) error {
			pPath, err := plistPath()
			if err != nil {
				return err
			}
			if _, err := os.Stat(pPath); os.IsNotExist(err) {
				fmt.Println("LaunchAgent: not installed (no plist).")
				fmt.Println("Run: assistclaw service install")
				return nil
			}
			fmt.Printf("plist: %s\n", pPath)
			uid := strconv.Itoa(os.Getuid())
			domain := "gui/" + uid
			out, err := exec.Command("launchctl", "print", domain+"/com.assistclaw.agent").CombinedOutput()
			if err != nil {
				fmt.Printf("launchctl: not loaded in %s (reboot or run: assistclaw service install)\n", domain)
				fmt.Printf("detail: %s\n", strings.TrimSpace(string(out)))
				return nil
			}
			fmt.Printf("launchctl: loaded in %s\n", domain)
			fmt.Println(strings.TrimSpace(string(out)))
			ld, _ := logDir()
			fmt.Printf("\nlog file: %s\n", filepath.Join(ld, "assistclaw.log"))
			return nil
		},
	}
}
