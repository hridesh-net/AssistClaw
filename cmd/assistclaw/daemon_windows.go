//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// CheckPID checks if the process with the given PID is running on Windows.
func CheckPID(pid int) bool {
	// On Windows, os.FindProcess always succeeds.
	// We use 'tasklist' to check if the process actually exists.
	cmd := exec.Command("tasklist", "/FI", fmt.Sprintf("PID eq %d", pid), "/NH")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	// 'tasklist' returns a line containing the PID if found.
	return strings.Contains(string(output), fmt.Sprintf("%d", pid))
}

// Detach re-executes the current process in the background with the provided arguments on Windows.
func Detach(args ...string) error {
	cmd := exec.Command(os.Args[0], args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	// On Windows, just starting the process without waiting is often sufficient
	// to "detach" it from the current terminal session if it handles its own signals.
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start background process: %w", err)
	}

	fmt.Printf("AssistClaw started in background (PID: %d)\n", cmd.Process.Pid)
	os.Exit(0)
	return nil
}
