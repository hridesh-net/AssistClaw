package bedrock

import (
	"testing"

	"github.com/assistclaw/assistclaw/internal/provider"
)

func TestMergeToolResultTurnsForConverse_multiToolOneUserTurn(t *testing.T) {
	in := []provider.Message{
		{Role: provider.RoleUser, Content: []provider.ContentPart{{Type: provider.ContentTypeText, Text: "hi"}}},
		{Role: provider.RoleAssistant, Content: []provider.ContentPart{
			{Type: provider.ContentTypeToolUse, ToolUseID: "a", ToolName: "read_file"},
			{Type: provider.ContentTypeToolUse, ToolUseID: "b", ToolName: "bash"},
			{Type: provider.ContentTypeToolUse, ToolUseID: "c", ToolName: "list_dir"},
			{Type: provider.ContentTypeToolUse, ToolUseID: "d", ToolName: "grep"},
		}},
		{Role: provider.RoleTool, Content: []provider.ContentPart{{Type: provider.ContentTypeToolResult, ToolResultID: "a", ToolResultContent: "1"}}},
		{Role: provider.RoleTool, Content: []provider.ContentPart{{Type: provider.ContentTypeToolResult, ToolResultID: "b", ToolResultContent: "2"}}},
		{Role: provider.RoleTool, Content: []provider.ContentPart{{Type: provider.ContentTypeToolResult, ToolResultID: "c", ToolResultContent: "3"}}},
		{Role: provider.RoleTool, Content: []provider.ContentPart{{Type: provider.ContentTypeToolResult, ToolResultID: "d", ToolResultContent: "4"}}},
	}
	out := mergeToolResultTurnsForConverse(in)
	if len(out) != 3 {
		t.Fatalf("got %d messages, want 3: %+v", len(out), out)
	}
	if out[2].Role != provider.RoleUser {
		t.Fatalf("third message role: %s", out[2].Role)
	}
	if len(out[2].Content) != 4 {
		t.Fatalf("merged tool results: got %d parts", len(out[2].Content))
	}
}

func TestMergeToolResultTurnsForConverse_preservesSingleTool(t *testing.T) {
	in := []provider.Message{
		{Role: provider.RoleUser, Content: []provider.ContentPart{{Type: provider.ContentTypeText, Text: "x"}}},
		{Role: provider.RoleAssistant, Content: []provider.ContentPart{
			{Type: provider.ContentTypeToolUse, ToolUseID: "only", ToolName: "bash"},
		}},
		{Role: provider.RoleTool, Content: []provider.ContentPart{{Type: provider.ContentTypeToolResult, ToolResultID: "only", ToolResultContent: "ok"}}},
	}
	out := mergeToolResultTurnsForConverse(in)
	if len(out) != 3 || len(out[2].Content) != 1 {
		t.Fatalf("unexpected: %+v", out)
	}
}

func TestMergeToolResultTurnsForConverse_noToolUsePassthrough(t *testing.T) {
	in := []provider.Message{
		{Role: provider.RoleUser, Content: []provider.ContentPart{{Type: provider.ContentTypeText, Text: "a"}}},
		{Role: provider.RoleAssistant, Content: []provider.ContentPart{{Type: provider.ContentTypeText, Text: "b"}}},
	}
	out := mergeToolResultTurnsForConverse(in)
	if len(out) != 2 {
		t.Fatalf("got %d want 2", len(out))
	}
}
