package email

import (
	"testing"

	"github.com/assistclaw/assistclaw/internal/config"
)

func TestActionForRuleOrderAndIgnore(t *testing.T) {
	rules := []config.EmailRuleConfig{
		{Match: config.EmailRuleMatch{FromDomain: "spam.test"}, Action: "ignore"},
		{Match: config.EmailRuleMatch{Subject: "^\\[VIP\\]"}, Action: "notify_only"},
	}
	m := &MailMessage{From: "x@spam.test", Subject: "[VIP] hello"}
	if got := ActionFor(rules, m); got != ActionIgnore {
		t.Fatalf("first rule should win: got %s", got)
	}
	m2 := &MailMessage{From: "friend@example.com", Subject: "[VIP] sale"}
	if got := ActionFor(rules, m2); got != ActionNotifyOnly {
		t.Fatalf("expected notify_only, got %s", got)
	}
}

func TestActionForDefaultWhenNoMatch(t *testing.T) {
	rules := []config.EmailRuleConfig{
		{Match: config.EmailRuleMatch{From: "only@me.com"}, Action: "ignore"},
	}
	m := &MailMessage{From: "other@me.com", Subject: "hi"}
	if got := ActionFor(rules, m); got != ActionAuto {
		t.Fatalf("expected default auto, got %s", got)
	}
}
