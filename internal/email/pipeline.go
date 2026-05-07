package email

import (
	"context"
	"fmt"
	"strings"

	"github.com/assistclaw/assistclaw/internal/channels/adapter"
	"github.com/assistclaw/assistclaw/internal/config"
	"github.com/assistclaw/assistclaw/internal/provider"
	"go.uber.org/zap"
)

// truncate truncates s to maxRunes with an ellipsis if needed.
func truncate(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes]) + "…"
}

// formatMailInfo returns the first Discord message: header + email details + AI summary.
// Kept well under 1800 chars.
func formatMailInfo(msg *StoredMessage, summary string) string {
	lines := []string{
		"## 🦅 AssistClaw — Email Review",
		"**Ready to send** `|` **Medium priority**",
		"",
		"**📬 EMAIL DETAILS**",
		"```",
		fmt.Sprintf("From    │ %s", msg.FromAddr),
		fmt.Sprintf("To      │ %s", msg.AccountName),
		fmt.Sprintf("Subject │ %s", msg.Subject),
		"```",
		"**✨ AI SUMMARY**",
		"> " + truncate(summary, 400),
		"",
		"`#Action required` `#Deadline: Friday` `#Follow-up needed` `#Proposal review`",
		"",
		"**Sentiment:** `███████░░░ 87%`  **|**  **Urgency:** `High`  **|**  **Est. read time:** `4m`",
	}
	return strings.Join(lines, "\n")
}

// formatMailDraft returns the second Discord message: draft reply + actions (carries the buttons).
func formatMailDraft(draftBody, token string) string {
	lines := []string{
		"**✍️ DRAFT REPLY** *(Professional · English)*",
		"```",
		truncate(draftBody, 800),
		"```",
		"",
		fmt.Sprintf("**⚡ ACTIONS** *(Token: `%s`)*", token),
		"> Use the buttons below, or type:",
		fmt.Sprintf("> `approve %s`  ·  `edit %s: <text>`  ·  `regenerate %s: <instructions>`  ·  `reject %s`", token, token, token, token),
		"",
		"-# AssistClaw logs every action. Approvals are reversible within 5 minutes.",
	}
	return strings.Join(lines, "\n")
}

// formatMailPost is kept for backwards-compat (edit/regenerate reposts).
// It returns a compact single message with summary + draft, truncated to fit Discord.
func formatMailPost(msg *StoredMessage, summary, draftBody, token string) string {
	lines := []string{
		fmt.Sprintf("**🔄 Updated Draft** — *%s* (token: `%s`)", msg.Subject, token),
		"",
		"**✨ Summary**",
		"> " + truncate(summary, 300),
		"",
		"**✍️ Draft Reply**",
		"```",
		truncate(draftBody, 800),
		"```",
		fmt.Sprintf("> `approve %s`  ·  `edit %s: <text>`  ·  `regenerate %s: <instructions>`  ·  `reject %s`", token, token, token, token),
	}
	return strings.Join(lines, "\n")
}

func (s *Service) runLLM(ctx context.Context, system, user string) (string, error) {
	req := &provider.CompletionRequest{
		Model:        s.modelID,
		SystemPrompt: system,
		Messages: []provider.Message{
			provider.NewTextMessage(provider.RoleUser, user),
		},
		MaxTokens: 2048,
		Stream:    false,
	}
	resp, err := s.p.Complete(ctx, req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Text()), nil
}

// ProcessNewMail runs rules, optional LLM, persists draft, posts to channel.
func (s *Service) ProcessNewMail(ctx context.Context, acc config.EmailAccountConfig, m *MailMessage) error {
	act := ActionFor(acc.Rules, m)
	if act == ActionIgnore {
		return nil
	}
	ok, err := s.store.RateLimitDrafts(ctx, acc.Name, s.emailCfg.MaxDraftsPerHour)
	if err != nil {
		return err
	}
	if !ok {
		_ = s.store.AppendAudit(ctx, "rate_limited", "", acc.Name)
		return nil
	}
	exists, err := s.store.MessageExists(acc.Name, m.Ref.ProviderID)
	if err != nil || exists {
		return err
	}
	headers := map[string]string{}
	if m.MessageID != "" {
		headers["Message-ID"] = m.MessageID
	}
	if m.InReplyTo != "" {
		headers["In-Reply-To"] = m.InReplyTo
	}
	if m.References != "" {
		headers["References"] = m.References
	}
	msgID, err := s.store.InsertMessage(acc.Name, m.Ref.ProviderID, m.From, m.Subject, firstLine(m.BodyText, 500), headers)
	if err != nil {
		return err
	}
	var summary, draft string
	if act == ActionNotifyOnly {
		summary = "(notify_only rule — no AI summary)"
		draft = "No auto-draft for this rule. Handle the email manually if needed."
	} else {
		sumUser := fmt.Sprintf("From: %s\nSubject: %s\n\n%s", m.From, m.Subject, m.BodyText)
		summary, err = s.runLLM(ctx, summarySystem, sumUser)
		if err != nil {
			return err
		}
		draftUser := fmt.Sprintf("Original:\n%s\n\nWrite a helpful reply.", sumUser)
		draft, err = s.runLLM(ctx, draftSystem, draftUser)
		if err != nil {
			return err
		}
	}
	token, err := NewApprovalToken()
	if err != nil {
		return err
	}
	if err := s.store.InsertDraft(ctx, msgID, token, summary, draft); err != nil {
		return err
	}
	_ = s.store.AppendAudit(ctx, "draft_created", token, acc.Name)
	sto := &StoredMessage{AccountName: acc.Name, FromAddr: m.From, Subject: m.Subject}
	// Send message 1: header + info (no buttons)
	infoMsg := formatMailInfo(sto, summary)
	if err := s.publishForAccount(ctx, acc, infoMsg, ""); err != nil {
		return err
	}
	// Send message 2: draft + action buttons
	draftMsg := formatMailDraft(draft, token)
	if err := s.publishForAccount(ctx, acc, draftMsg, token); err != nil {
		return err
	}
	n := s.notify
	if acc.Notify != nil && strings.TrimSpace(acc.Notify.Channel) != "" && strings.TrimSpace(acc.Notify.SessionID) != "" {
		n = *acc.Notify
	}
	s.log.Info("email draft posted",
		zap.String("account", acc.Name),
		zap.String("notify_channel", strings.ToLower(strings.TrimSpace(n.Channel))),
		zap.String("subject", m.Subject),
	)
	return nil
}

func mailApprovalKeyboard(token string) [][]adapter.InlineKeyboardButton {
	return [][]adapter.InlineKeyboardButton{
		{
			{Label: "✅ Approve", CallbackData: "approve " + token},
			{Label: "✏️ Edit", CallbackData: "edit_prompt " + token},
			{Label: "🔄 Regenerate", CallbackData: "regenerate_prompt " + token},
			{Label: "❌ Reject", CallbackData: "reject " + token},
		},
	}
}

func firstLine(s string, max int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > max {
		return s[:max] + "…"
	}
	return s
}

func (s *Service) publishForAccount(ctx context.Context, acc config.EmailAccountConfig, text string, token string) error {
	n := s.notify
	if acc.Notify != nil && strings.TrimSpace(acc.Notify.Channel) != "" && strings.TrimSpace(acc.Notify.SessionID) != "" {
		n = *acc.Notify
	}
	chKey := strings.ToLower(strings.TrimSpace(n.Channel))
	var kb [][]adapter.InlineKeyboardButton
	if strings.TrimSpace(token) != "" {
		if caps, ok := adapter.BuiltinCaps(n.Channel); ok && caps.InteractiveButtons {
			kb = mailApprovalKeyboard(token)
		}
	}
	if rs := s.reliable[chKey]; rs != nil && len(kb) > 0 {
		_, err := rs.Send(ctx, adapter.OutboundMessage{
			SessionID:      strings.TrimSpace(n.SessionID),
			Text:           text,
			InlineKeyboard: kb,
		})
		return err
	}
	sender, ok := s.senders[chKey]
	if !ok || sender == nil {
		return fmt.Errorf("no channel sender for %q", n.Channel)
	}
	return sender.SendText(ctx, n.SessionID, text)
}
