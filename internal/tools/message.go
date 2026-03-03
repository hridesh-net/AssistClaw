package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/assistclaw/assistclaw/internal/provider"
)

// ChannelSender is the interface that active channels must implement
// to support proactive outbound messages from the agent.
type ChannelSender interface {
	SendText(ctx context.Context, sessionID, text string) error
}

// MessageTool lets the agent proactively send a message to a connected channel.
type MessageTool struct {
	// Senders is a map of channelID → ChannelSender (registered at startup).
	Senders map[string]ChannelSender
}

func (t MessageTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name: "message",
		Description: `Send a proactive message to a connected messaging channel (WhatsApp, Telegram, Slack, Discord).
Use this to notify the user of completed tasks, send reminders, or share results without waiting to be asked.
If no channel is specified, sends to all active channels.`,
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"text": map[string]any{
					"type":        "string",
					"description": "Message text to send",
				},
				"channel": map[string]any{
					"type":        "string",
					"description": "Channel to send to (e.g. 'whatsapp', 'telegram', 'slack', 'discord'). Omit to send to all.",
				},
				"session_id": map[string]any{
					"type":        "string",
					"description": "Session ID / recipient to send to (optional)",
				},
			},
			Required: []string{"text"},
		},
	}
}

func (t MessageTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		Text      string `json:"text"`
		Channel   string `json:"channel"`
		SessionID string `json:"session_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.Text == "" {
		return "message: 'text' is required", nil
	}

	if len(t.Senders) == 0 {
		return "message: no channels with outbound send support are active. Configure channels during onboarding.", nil
	}

	var sent []string
	var failed []string

	for id, sender := range t.Senders {
		if args.Channel != "" && !strings.EqualFold(id, args.Channel) {
			continue
		}
		if err := sender.SendText(ctx, args.SessionID, args.Text); err != nil {
			failed = append(failed, fmt.Sprintf("%s: %v", id, err))
		} else {
			sent = append(sent, id)
		}
	}

	if args.Channel != "" && len(sent) == 0 && len(failed) == 0 {
		names := make([]string, 0, len(t.Senders))
		for k := range t.Senders {
			names = append(names, k)
		}
		return fmt.Sprintf("Channel %q does not support outbound sends. Active senders: %s",
			args.Channel, strings.Join(names, ", ")), nil
	}

	var parts []string
	if len(sent) > 0 {
		parts = append(parts, fmt.Sprintf("✔ Sent to: %s", strings.Join(sent, ", ")))
	}
	if len(failed) > 0 {
		parts = append(parts, fmt.Sprintf("✗ Failed: %s", strings.Join(failed, "; ")))
	}
	return strings.Join(parts, "\n"), nil
}
