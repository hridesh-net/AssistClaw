package main

import (
	"strings"
	"testing"
)

func TestSanitizeDoctorMessage_OpenAIKey(t *testing.T) {
	t.Parallel()
	in := `request failed: sk-1234567890abcdefghijklmnop`
	out := sanitizeDoctorMessage(in)
	if strings.Contains(out, "sk-1234567890") {
		t.Fatalf("expected key redacted, got %q", out)
	}
	if !strings.Contains(out, "sk-…") {
		t.Fatalf("expected placeholder, got %q", out)
	}
}

func TestSanitizeDoctorMessage_Bearer(t *testing.T) {
	t.Parallel()
	in := `Authorization: Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U`
	out := sanitizeDoctorMessage(in)
	if strings.Contains(out, "eyJ") {
		t.Fatalf("expected bearer redacted, got %q", out)
	}
	if !strings.Contains(out, "Bearer …") {
		t.Fatalf("expected Bearer placeholder, got %q", out)
	}
}

func TestFormatProviderHealthMessage_Hints(t *testing.T) {
	t.Parallel()
	m := formatProviderHealthMessage("openai", "openai: list models failed: 401 Unauthorized")
	if !strings.Contains(m, "401") && !strings.Contains(m, "Unauthorized") {
		t.Fatalf("expected auth context: %q", m)
	}
	if !strings.Contains(m, "authentication") {
		t.Fatalf("expected hint: %q", m)
	}
}
