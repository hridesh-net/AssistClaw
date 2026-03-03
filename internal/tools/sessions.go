package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/memory"
	"github.com/assistclaw/assistclaw/internal/provider"
)

// SessionsListTool lists all sessions stored in episodic memory.
type SessionsListTool struct {
	Episodic *memory.EpisodicMemory
}

func (t SessionsListTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "sessions_list",
		Description: "List all past conversation session IDs stored in episodic memory. Use this to find sessions to review with sessions_history.",
		InputSchema: provider.ToolParameter{
			Type:       "object",
			Properties: map[string]any{},
			Required:   []string{},
		},
	}
}

func (t SessionsListTool) Execute(ctx context.Context, _ json.RawMessage) (string, error) {
	if t.Episodic == nil {
		return "sessions_list: episodic memory not initialised", nil
	}
	sessions, err := t.Episodic.ListSessions(ctx, 50)
	if err != nil {
		return fmt.Sprintf("sessions_list: %v", err), nil
	}

	if len(sessions) == 0 {
		return "No sessions found in episodic memory.", nil
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Found %d session(s):\n", len(sessions)))
	for _, s := range sessions {
		sb.WriteString("  • " + s + "\n")
	}
	return sb.String(), nil
}

// SessionsHistoryTool reads the conversation history for a specific session.
type SessionsHistoryTool struct {
	Episodic interface {
		GetSession(ctx context.Context, sessionID string, limit int) ([]memory.Message, error)
	}
}

func (t SessionsHistoryTool) Definition() provider.ToolDef {
	return provider.ToolDef{
		Name:        "sessions_history",
		Description: "Read the conversation history for a specific session ID from episodic memory. Returns messages in chronological order.",
		InputSchema: provider.ToolParameter{
			Type: "object",
			Properties: map[string]any{
				"session_id": map[string]any{"type": "string", "description": "Session ID to retrieve history for"},
				"limit":      map[string]any{"type": "integer", "description": "Max messages to return (default 20)"},
			},
			Required: []string{"session_id"},
		},
	}
}

func (t SessionsHistoryTool) Execute(ctx context.Context, input json.RawMessage) (string, error) {
	var args struct {
		SessionID string `json:"session_id"`
		Limit     int    `json:"limit"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "", err
	}
	if args.Limit <= 0 {
		args.Limit = 20
	}
	if t.Episodic == nil {
		return "sessions_history: episodic memory not initialised", nil
	}

	msgs, err := t.Episodic.GetSession(ctx, args.SessionID, args.Limit)
	if err != nil {
		return fmt.Sprintf("sessions_history: %v", err), nil
	}
	if len(msgs) == 0 {
		return fmt.Sprintf("No messages found for session %q.", args.SessionID), nil
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("=== Session %s (%d messages) ===\n\n", args.SessionID, len(msgs)))
	for _, m := range msgs {
		ts := m.CreatedAt.Format(time.RFC3339)
		role := string(m.Role)
		content := m.Content
		if len(content) > 500 {
			content = content[:497] + "…"
		}
		sb.WriteString(fmt.Sprintf("[%s] %s: %s\n\n", ts, strings.ToUpper(role), content))
	}
	return sb.String(), nil
}
