package email

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var (
	reApprove = regexp.MustCompile(`(?i)^\s*approve\s+([a-z0-9]+)\s*$`)
	reReject  = regexp.MustCompile(`(?i)^\s*reject\s+([a-z0-9]+)\s*$`)
	reEdit    = regexp.MustCompile(`(?i)^\s*edit\s+([a-z0-9]+)\s*:\s*(.+)$`)
)

// ParseInboundCommand detects approve/reject/edit lines.
func ParseInboundCommand(text string) (verb, token, editBody string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", "", ""
	}
	// Single-line only for safety
	first := strings.Split(text, "\n")[0]
	first = strings.TrimSpace(first)
	if m := reApprove.FindStringSubmatch(first); len(m) == 2 {
		return "approve", strings.ToLower(m[1]), ""
	}
	if m := reReject.FindStringSubmatch(first); len(m) == 2 {
		return "reject", strings.ToLower(m[1]), ""
	}
	if m := reEdit.FindStringSubmatch(first); len(m) == 3 {
		return "edit", strings.ToLower(m[1]), strings.TrimSpace(m[2])
	}
	return "", "", ""
}

// NewApprovalToken returns a short random token (8 hex chars).
func NewApprovalToken() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

// HandleInboundCommand processes approve/reject/edit; returns reply message and handled flag.
func (s *Service) HandleInboundCommand(ctx context.Context, channelID, sessionID, text string) (reply string, handled bool, err error) {
	if s == nil || s.store == nil {
		return "", false, nil
	}
	if !s.inboundMatchesNotify(channelID, sessionID) {
		return "", false, nil
	}
	verb, token, editBody := ParseInboundCommand(text)
	if verb == "" {
		return "", false, nil
	}
	if strings.Contains(strings.ToLower(text), "delete") {
		return "Email deletion is disabled by design. I can only send a reply after you approve a draft — I cannot delete or modify messages in your mailbox.", true, nil
	}
	d, msg, headers, err := s.store.GetDraftByToken(ctx, token)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Sprintf("Unknown or expired approval token %q.", token), true, nil
		}
		return "", true, err
	}
	if d == nil {
		return fmt.Sprintf("Unknown or expired approval token %q.", token), true, nil
	}
	if d.Status != DraftPending {
		return fmt.Sprintf("Token %q is no longer pending (status: %s).", token, d.Status), true, nil
	}

	switch verb {
	case "reject":
		_ = s.store.SetDraftStatus(ctx, token, DraftRejected)
		_ = s.store.AppendAudit(ctx, "reject", token, msg.Subject)
		if goalID, _ := s.store.GoalIDForDraft(ctx, token); goalID > 0 {
			// Reset the silence timer so the follow-up loop doesn't immediately re-draft.
			_ = s.store.TouchGoal(ctx, goalID)
			return fmt.Sprintf("Rejected goal draft %s. Goal #%d stays open — edit the next draft, handle the thread manually, or cancel with: assistclaw goal cancel %d", token, goalID, goalID), true, nil
		}
		return fmt.Sprintf("Rejected draft %s. You can handle the thread manually.", token), true, nil

	case "edit":
		if editBody == "" {
			return "Edit text is empty; use: edit <token>: <new body>", true, nil
		}
		if err := s.store.UpdateDraftBody(ctx, token, editBody); err != nil {
			return "", true, err
		}
		_ = s.store.AppendAudit(ctx, "edit", token, editBody)
		// Repost updated draft
		body := formatMailPost(msg, d.Summary, editBody, token)
		acc, ok := s.accountByName(msg.AccountName)
		if !ok {
			return "Internal error: unknown account " + msg.AccountName, true, nil
		}
		if err := s.publishForAccount(ctx, acc, body, token); err != nil {
			return "Updated draft locally but failed to post: " + err.Error(), true, nil
		}
		return "Draft updated and reposted. Reply with approve " + token + " when ready.", true, nil

	case "approve":
		_ = s.store.AppendAudit(ctx, "approve_intent", token, msg.Subject)
		if goalID, gerr := s.store.GoalIDForDraft(ctx, token); gerr == nil && goalID > 0 {
			replyText, err := s.approveGoalDraft(ctx, goalID, token, d, msg, headers)
			if err != nil {
				return "", true, err
			}
			return replyText, true, nil
		}
		be := s.backends[msg.AccountName]
		if be == nil {
			return "Internal error: no backend for account " + msg.AccountName, true, nil
		}
		mm := &MailMessage{
			Ref:        Ref{AccountName: msg.AccountName, ProviderID: msg.ProviderMsgID},
			From:       msg.FromAddr,
			Subject:    msg.Subject,
			BodyText:   msg.Snippet,
			MessageID:  headers["Message-ID"],
			InReplyTo:  headers["In-Reply-To"],
			References: headers["References"],
		}
		if err := be.Reply(ctx, mm, d.Body); err != nil {
			_ = s.store.AppendAudit(ctx, "send_failed", token, err.Error())
			return "Failed to send reply: " + err.Error(), true, nil
		}
		_ = s.store.SetDraftStatus(ctx, token, DraftSent)
		_ = s.store.AppendAudit(ctx, "send_ok", token, "")
		return fmt.Sprintf("Sent approved reply for token %s.", token), true, nil
	default:
		return "", false, nil
	}
}
