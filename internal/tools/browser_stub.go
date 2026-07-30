//go:build !assistclaw_browser

package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// BrowserNavigate is a no-op stub when the browser tag is not set.
type BrowserNavigate struct{}

func (b BrowserNavigate) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "browser_navigate",
		Description: "Opens a URL in a headless browser and extracts the visible text content. (disabled at build time)",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"url": map[string]any{"type": "string", "description": "URL to open"},
			},
			Required: []string{"url"},
		},
	}
}

func (b BrowserNavigate) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "", fmt.Errorf("browser tools are disabled; rebuild with -tags assistclaw_browser")
}

// BrowserScreenshot is a no-op stub when the browser tag is not set.
type BrowserScreenshot struct{}

func (b BrowserScreenshot) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "browser_screenshot",
		Description: "Opens a URL in a headless browser and takes a full-page screenshot. (disabled at build time)",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"url":     map[string]any{"type": "string", "description": "URL to capture"},
				"quality": map[string]any{"type": "integer", "description": "JPEG quality (1-100)"},
			},
			Required: []string{"url"},
		},
	}
}

func (b BrowserScreenshot) Execute(_ context.Context, _ json.RawMessage) (string, error) {
	return "", fmt.Errorf("browser tools are disabled; rebuild with -tags assistclaw_browser")
}
