package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/assistclaw/assistclaw/internal/channels"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// SendMediaTool allows the agent to send files back to the user via the messaging channel.
type SendMediaTool struct {
	// No longer needs MediaFn field, pulls from context
}

func (t SendMediaTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "send_media",
		Description: `Send an image or file back to the user.
Supports PNG, JPEG, WEBP, and general documents.
Use this to show generated images, graphs, or requested files.`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Local path to the file to send",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Optional caption to send with the media",
				},
			},
			Required: []string{"path"},
		},
	}
}

func (t SendMediaTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Path    string `json:"path"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}

	// Pull mediaFn from context
	val := ctx.Value(channels.MediaFnKey)
	if val == nil {
		return "send_media: media output is not supported on this channel", nil
	}
	mediaFn, ok := val.(channels.MediaReplyFunc)
	if !ok {
		return "send_media: internal error (invalid media callback)", nil
	}

	data, err := os.ReadFile(args.Path)
	if err != nil {
		return fmt.Sprintf("send_media: failed to read file: %v", err), nil
	}

	// Infer MIME type
	mimeType := "application/octet-stream"
	lower := strings.ToLower(args.Path)
	if strings.HasSuffix(lower, ".png") {
		mimeType = "image/png"
	} else if strings.HasSuffix(lower, ".jpg") || strings.HasSuffix(lower, ".jpeg") {
		mimeType = "image/jpeg"
	} else if strings.HasSuffix(lower, ".webp") {
		mimeType = "image/webp"
	} else if strings.HasSuffix(lower, ".pdf") {
		mimeType = "application/pdf"
	}

	err = mediaFn(data, args.Path, mimeType)
	if err != nil {
		return fmt.Sprintf("send_media: failed to send: %v", err), nil
	}

	return fmt.Sprintf("Successfully sent %s to the user.", args.Path), nil
}
