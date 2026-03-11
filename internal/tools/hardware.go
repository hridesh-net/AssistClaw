package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/assistclaw/assistclaw/internal/provider"
	"github.com/assistclaw/assistclaw/internal/system"
)

// ListHardwareTool re-scans for connected hardware devices.
type ListHardwareTool struct{}

func (ListHardwareTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "list_hardware",
		Description: "Scan for connected hardware devices (cameras, microphones, input devices).",
		InputSchema: provider.ToolParameter{
			Type:       "object",
			Properties: map[string]any{},
		},
	}
}

func (ListHardwareTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	report, err := system.Detect(ctx)
	if err != nil {
		return "", err
	}
	report.LastUpdated = time.Now().Format(time.RFC3339)

	data, _ := json.MarshalIndent(report, "", "  ")
	return string(data), nil
}
