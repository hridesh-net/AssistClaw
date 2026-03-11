package system

import (
	"context"
	"os/exec"
	"strings"
)

// HardwareReport contains details about detected hardware devices.
type HardwareReport struct {
	AudioDevices []string `json:"audio_devices"`
	Cameras      []string `json:"cameras"`
	InputDevices []string `json:"input_devices"`
	LastUpdated  string   `json:"last_updated"`
}

// Detect performs hardware discovery on the host system.
// Currently optimized for macOS using system_profiler.
func Detect(ctx context.Context) (*HardwareReport, error) {
	report := &HardwareReport{}

	// Detect Audio
	audio, _ := runCommand(ctx, "system_profiler", "SPAudioDataType")
	report.AudioDevices = parseSystemProfiler(audio, "Devices:")

	// Detect Cameras
	cameras, _ := runCommand(ctx, "system_profiler", "SPCameraDataType")
	report.Cameras = parseSystemProfiler(cameras, "Camera:")

	// Detect Input Devices (USB)
	usb, _ := runCommand(ctx, "system_profiler", "SPUSBDataType")
	report.InputDevices = parseUSBDevices(usb)

	return report, nil
}

func runCommand(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// parseSystemProfiler extract device names from indented system_profiler output.
func parseSystemProfiler(input, sectionHeader string) []string {
	var devices []string
	lines := strings.Split(input, "\n")
	inSection := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == sectionHeader {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") {
			name := strings.TrimSuffix(trimmed, ":")
			if name != "" {
				devices = append(devices, name)
			}
		}
	}
	return devices
}

func parseUSBDevices(input string) []string {
	var devices []string
	lines := strings.Split(input, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "        ") && !strings.HasPrefix(line, "          ") {
			name := strings.TrimSpace(line)
			name = strings.TrimSuffix(name, ":")
			if name != "" && !strings.Contains(name, "Hub") && !strings.Contains(name, "Controller") {
				devices = append(devices, name)
			}
		}
	}
	return devices
}
