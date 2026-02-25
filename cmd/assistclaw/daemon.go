package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
