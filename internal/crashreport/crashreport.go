package crashreport

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"go.uber.org/zap"
)

// Report captures information about a daemon crash.
type Report struct {
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
	GoVersion string `json:"go_version"`
	OSArch    string `json:"os_arch"`
	Panic     string `json:"panic"`
	Stack     string `json:"stack"`
}

// sensitivePattern matches env vars and auth tokens in stack traces.
var sensitivePattern = regexp.MustCompile(`(?i)(\b[A-Z_]*(?:TOKEN|KEY|SECRET|PASSWORD|AUTH|CREDENTIAL)[A-Z_]*\b|=)([^\s&;]+)`)

// redact replaces sensitive values with [REDACTED].
func redact(input string) string {
	return sensitivePattern.ReplaceAllString(input, `${1}[REDACTED]`)
}

// Write saves a crash report to the state directory.
func Write(stateDir, version string, panicValue any) (string, error) {
	dir := filepath.Join(stateDir, "crashes")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir crashes: %w", err)
	}

	r := Report{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   version,
		GoVersion: fmt.Sprintf("%s %s/%s", runtime.Version(), runtime.GOOS, runtime.GOARCH),
		OSArch:    fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
		Panic:     redact(fmt.Sprintf("%v", panicValue)),
		Stack:     redact(string(debug.Stack())),
	}

	name := fmt.Sprintf("crash-%s.json", time.Now().UTC().Format("20060102-150405"))
	path := filepath.Join(dir, name)
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o640); err != nil {
		return "", err
	}
	return path, nil
}

// ScanPending returns paths of crash reports that haven't been sent yet.
func ScanPending(stateDir string) ([]string, error) {
	dir := filepath.Join(stateDir, "crashes")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pending []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".json") {
			pending = append(pending, filepath.Join(dir, e.Name()))
		}
	}
	return pending, nil
}

// MarkSent moves a crash report to the sent subdirectory.
func MarkSent(path string) error {
	dir := filepath.Join(filepath.Dir(path), "sent")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.Rename(path, filepath.Join(dir, filepath.Base(path)))
}

// Recover catches panics in long-lived goroutines and writes crash reports.
func Recover(stateDir, version string, log *zap.Logger) {
	if r := recover(); r != nil {
		path, err := Write(stateDir, version, r)
		if err != nil {
			log.Error("failed to write crash report", zap.Error(err))
		} else {
			log.Error("crash report written", zap.String("path", path), zap.Any("panic", r))
		}
		// Re-panic so systemd sees the failure and restarts.
		panic(r)
	}
}
