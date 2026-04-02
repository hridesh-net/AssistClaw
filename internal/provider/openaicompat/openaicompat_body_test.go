package openaicompat

import (
	"encoding/json"
	"testing"

	"github.com/assistclaw/assistclaw/internal/provider"
)

func TestBuildBody_emptyMessageContent(t *testing.T) {
	p := New(Config{Name: "test", BaseURL: "http://localhost:11434"})
	req := &provider.CompletionRequest{
		Model: "m",
		Messages: []provider.Message{
			{Role: provider.RoleUser, Content: nil},
		},
	}
	body, err := p.buildBody(req, false)
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Messages []struct {
			Content any `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Messages) != 1 {
		t.Fatalf("messages: %d", len(parsed.Messages))
	}
	if s, ok := parsed.Messages[0].Content.(string); !ok || s != "" {
		t.Fatalf("want empty string content, got %#v", parsed.Messages[0].Content)
	}
}

func TestMergeStaticAndAPIModels(t *testing.T) {
	api := []provider.ModelInfo{{ID: "b", Name: "b", Provider: "p"}}
	static := []provider.ModelInfo{{ID: "a", Name: "a", Provider: "p"}, {ID: "b", Name: "dup", Provider: "p"}}
	out := mergeStaticAndAPIModels(static, api, "p")
	if len(out) != 2 {
		t.Fatalf("len=%d %+v", len(out), out)
	}
}
