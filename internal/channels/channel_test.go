package channels

import (
	"testing"

	"github.com/assistclaw/assistclaw/internal/provider"
)

func TestSlashCommandLine(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		msg  Message
		want string
	}{
		{
			name: "plain",
			msg:  Message{Text: "/new"},
			want: "/new",
		},
		{
			name: "metadata prefix",
			msg:  Message{Text: "[CHAT INFO] channel: WhatsApp\n/new"},
			want: "/new",
		},
		{
			name: "quoted block then command",
			msg:  Message{Text: "> prior\n/new"},
			want: "/new",
		},
		{
			name: "text from parts when Text empty",
			msg: Message{
				Text: "",
				Parts: []provider.ContentPart{
					{Type: provider.ContentTypeText, Text: "  /reset  "},
				},
			},
			want: "/reset",
		},
		{
			name: "no slash line",
			msg:  Message{Text: "hello /new"},
			want: "",
		},
		{
			name: "bom and leading marks",
			msg:  Message{Text: "\ufeff\n/new"},
			want: "/new",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := SlashCommandLine(tt.msg); got != tt.want {
				t.Fatalf("SlashCommandLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
