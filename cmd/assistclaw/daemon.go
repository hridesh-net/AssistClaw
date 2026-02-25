package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// PidFile returns the path to the PID file in the given state directory.
func PidFile(stateDir string) string {
	return filepath.Join(stateDir, "assistclaw.pid")
}

// WritePID writes the current process ID to the PID file.
func WritePID(path string) error {
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

// ReadPID reads the PID from the PID file.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(data)))
}

// CheckPID checks if the process with the given PID is running.
func CheckPID(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds. Sending signal 0 checks if it exists.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// Detach re-executes the current process in the background with the provided arguments.
func Detach(args ...string) error {
	cmd := exec.Command(os.Args[0], args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	cmd.Stdin = nil

	// Set SysProcAttr to detach from terminal
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start background process: %w", err)
	}

	fmt.Printf("AssistClaw started in background (PID: %d)\n", cmd.Process.Pid)
	os.Exit(0)
	return nil
}
