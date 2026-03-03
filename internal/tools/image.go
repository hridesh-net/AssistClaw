package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// ImageUnderstandTool passes an image to a vision-capable LLM and returns the description.
type ImageUnderstandTool struct {
	Provider provider.Provider
	Model    string
}

func (t ImageUnderstandTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "image_understand",
		Description: `Analyze an image file and return a detailed description of its contents.
Accepts a file path (PNG, JPEG, WEBP, GIF) or a base64-encoded image string.
Use after browser_screenshot to reason about what the browser shows.
Also useful for: reading error messages from screenshots, OCR, reviewing UI designs.`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the image file to analyze",
				},
				"base64_image": map[string]any{
					"type":        "string",
					"description": "Base64-encoded image data (alternative to path)",
				},
				"media_type": map[string]any{
					"type":        "string",
					"description": "MIME type (e.g. image/png, image/jpeg). Inferred from path if not provided.",
				},
				"question": map[string]any{
					"type":        "string",
					"description": "Specific question about the image. If omitted, provides a full description.",
				},
			},
			Required: []string{},
		},
	}
}

func (t ImageUnderstandTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path        string `json:"path"`
		Base64Image string `json:"base64_image"`
		MediaType   string `json:"media_type"`
		Question    string `json:"question"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}

	var rawBytes []byte
	var mediaType string

	switch {
	case args.Path != "":
		raw, err := os.ReadFile(args.Path)
		if err != nil {
			return fmt.Sprintf("image_understand: cannot read %s: %v", args.Path, err), nil
		}
		rawBytes = raw
		mediaType = inferMediaType(args.Path)
	case args.Base64Image != "":
		decoded, err := base64.StdEncoding.DecodeString(args.Base64Image)
		if err != nil {
			// Try raw (no padding)
			decoded, err = base64.RawStdEncoding.DecodeString(args.Base64Image)
			if err != nil {
				return "image_understand: invalid base64_image encoding", nil
			}
		}
		rawBytes = decoded
		mediaType = "image/png"
	default:
		return "image_understand: provide either 'path' or 'base64_image'", nil
	}

	if args.MediaType != "" {
		mediaType = args.MediaType
	}

	prompt := "Describe this image in detail. Include all visible text, UI elements, colors, layout, and any notable content."
	if args.Question != "" {
		prompt = args.Question
	}

	if t.Provider == nil {
		return fmt.Sprintf("Image loaded (%s, %d bytes). No vision provider configured — set a vision-capable model.", mediaType, len(rawBytes)), nil
	}

	req := &provider.CompletionRequest{
		Model: t.Model,
		Messages: []provider.Message{
			{
				Role: provider.RoleUser,
				Content: []provider.ContentPart{
					{
						Type:          provider.ContentTypeImage,
						ImageData:     rawBytes,
						ImageMimeType: mediaType,
					},
					{
						Type: provider.ContentTypeText,
						Text: prompt,
					},
				},
			},
		},
		MaxTokens: 1024,
	}

	resp, err := t.Provider.Complete(ctx, req)
	if err != nil {
		return fmt.Sprintf("image_understand: vision request failed: %v", err), nil
	}
	return resp.Text(), nil
}

// inferMediaType returns a MIME type based on file extension.
func inferMediaType(path string) string {
	lower := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg"):
		return "image/jpeg"
	case strings.HasSuffix(lower, ".gif"):
		return "image/gif"
	case strings.HasSuffix(lower, ".webp"):
		return "image/webp"
	default:
		return "image/png"
	}
}
