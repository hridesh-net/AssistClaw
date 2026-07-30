package email

import (
	"context"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/assistclaw/assistclaw/internal/config"
	"go.uber.org/zap"
)

// goalEventBodyLimit bounds how much message text we keep per transcript entry.
const goalEventBodyLimit = 3000

// goalAnchorProviderID is the synthetic email_messages row that anchors
// goal-initiated drafts (openers and follow-ups) which have no inbound mail.
func goalAnchorProviderID(goalID int64) string {
	return fmt.Sprintf("goal:%d:anchor", goalID)
}

func extractAddr(from string) string {
	if a, err := mail.ParseAddress(from); err == nil {
		return strings.ToLower(a.Address)
	}
	return strings.ToLower(strings.Trim(strings.TrimSpace(from), "<>"))
}

// splitMsgIDs splits an In-Reply-To/References header into individual Message-IDs.
func splitMsgIDs(headers ...string) []string {
	var out []string
	for _, h := range headers {
		for _, f := range strings.Fields(h) {
			if f = strings.TrimSpace(f); f != "" {
				out = append(out, f)
			}
		}
	}
	return out
}

func truncateForEvent(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > goalEventBodyLimit {
		return s[:goalEventBodyLimit] + "…"
	}
	return s
}

// OpenGoal creates a goal, drafts the opening email via the LLM, and posts it
// for approval. The opener is NOT sent until the user approves the token.
// Publishing to the notify channel is best-effort (the CLI has no senders);
// the returned token and draft let callers surface it themselves.
func (s *Service) OpenGoal(ctx context.Context, accountName, to, subject, objective string, followupAfter time.Duration, maxFollowups int) (*Goal, string, string, error) {
	acc, ok := s.accountByName(accountName)
	if !ok {
		return nil, "", "", fmt.Errorf("unknown email account %q", accountName)
	}
	be := s.backends[accountName]
	if be == nil {
		return nil, "", "", fmt.Errorf("no backend for account %q", accountName)
	}
	if _, ok := be.(NewMailSender); !ok {
		return nil, "", "", fmt.Errorf("backend %q cannot send new mail (goals need an SMTP-capable backend)", be.Name())
	}
	counterpart := extractAddr(to)
	if counterpart == "" || !strings.Contains(counterpart, "@") {
		return nil, "", "", fmt.Errorf("invalid counterpart address %q", to)
	}
	user := fmt.Sprintf("Objective: %s\nRecipient: %s\nSubject of the thread: %s\n\nWrite the opening email.", objective, counterpart, subject)
	draft, err := s.runLLM(ctx, goalOpenerSystem, user)
	if err != nil {
		return nil, "", "", fmt.Errorf("draft opener: %w", err)
	}
	g := &Goal{
		AccountName:   accountName,
		Counterpart:   counterpart,
		Subject:       subject,
		Objective:     objective,
		Status:        GoalActive,
		FollowupAfter: followupAfter,
		MaxFollowups:  maxFollowups,
	}
	if _, err := s.store.InsertGoal(ctx, g); err != nil {
		return nil, "", "", err
	}
	anchorID, err := s.store.InsertMessage(accountName, goalAnchorProviderID(g.ID), counterpart, subject, firstLine(objective, 500), map[string]string{})
	if err != nil {
		return nil, "", "", err
	}
	token, err := NewApprovalToken()
	if err != nil {
		return nil, "", "", err
	}
	if err := s.store.InsertGoalDraft(ctx, g.ID, anchorID, token, "Opening email for goal: "+objective, draft); err != nil {
		return nil, "", "", err
	}
	_ = s.store.AppendAudit(ctx, "goal_created", token, fmt.Sprintf("goal=%d %s", g.ID, objective))
	post := formatGoalPost(g, "Opening email drafted — approve to send.", draft, token)
	if err := s.publishForAccount(ctx, acc, post, token); err != nil {
		s.log.Info("goal opener drafted but not posted to channel (approve via CLI)",
			zap.Int64("goal", g.ID), zap.String("token", token), zap.Error(err))
	}
	return g, token, draft, nil
}

// maybeHandleGoalInbound routes an inbound mail to its goal, if any.
// Returns handled=true when the mail belonged to a goal thread.
func (s *Service) maybeHandleGoalInbound(ctx context.Context, acc config.EmailAccountConfig, m *MailMessage) (bool, error) {
	refs := splitMsgIDs(m.InReplyTo, m.References)
	g, err := s.store.FindActiveGoalForInbound(ctx, acc.Name, extractAddr(m.From), refs)
	if err != nil || g == nil {
		return false, err
	}
	return true, s.handleGoalInbound(ctx, acc, m, g)
}

// handleGoalInbound assesses a counterpart reply against the objective and
// either closes the goal, flags it blocked, or drafts the next reply.
func (s *Service) handleGoalInbound(ctx context.Context, acc config.EmailAccountConfig, m *MailMessage, g *Goal) error {
	exists, err := s.store.MessageExists(acc.Name, m.Ref.ProviderID)
	if err != nil {
		return err
	}
	if exists {
		return nil
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
	msgRowID, err := s.store.InsertMessage(acc.Name, m.Ref.ProviderID, m.From, m.Subject, firstLine(m.BodyText, 500), headers)
	if err != nil {
		return err
	}
	_ = s.store.AddGoalThreadRef(ctx, g.ID, m.MessageID)
	_ = s.store.AppendGoalEvent(ctx, g.ID, "inbound", truncateForEvent(fmt.Sprintf("From: %s\n%s", m.From, m.BodyText)))

	transcript, err := s.goalTranscript(ctx, g.ID)
	if err != nil {
		return err
	}
	user := fmt.Sprintf("Objective: %s\nCounterpart: %s\nThread subject: %s\n\nConversation so far:\n%s\n\nNewest message from counterpart:\n%s",
		g.Objective, g.Counterpart, g.Subject, transcript, truncateForEvent(m.BodyText))
	out, err := s.runLLM(ctx, goalReplySystem, user)
	if err != nil {
		return err
	}
	verdict, body := parseGoalAssessment(out)

	switch verdict {
	case "ACHIEVED":
		if err := s.store.SetGoalStatus(ctx, g.ID, GoalAchieved, body); err != nil {
			return err
		}
		_ = s.store.AppendAudit(ctx, "goal_achieved", "", fmt.Sprintf("goal=%d", g.ID))
		note := fmt.Sprintf("🎯 [Goal #%d achieved] %s\n\n%s\nNo further action needed.", g.ID, g.Objective, body)
		if err := s.publishForAccount(ctx, acc, note, ""); err != nil {
			s.log.Warn("goal achieved but notify failed", zap.Int64("goal", g.ID), zap.Error(err))
		}
		return nil

	case "BLOCKED":
		if err := s.store.SetGoalStatus(ctx, g.ID, GoalBlocked, body); err != nil {
			return err
		}
		note := fmt.Sprintf("⛔ [Goal #%d needs you] %s\n\n%s\nReply on the thread yourself, or cancel with: assistclaw goal cancel %d", g.ID, g.Objective, body, g.ID)
		if err := s.publishForAccount(ctx, acc, note, ""); err != nil {
			s.log.Warn("goal blocked but notify failed", zap.Int64("goal", g.ID), zap.Error(err))
		}
		return nil

	default: // CONTINUE
		ok, err := s.store.RateLimitDrafts(ctx, acc.Name, s.emailCfg.MaxDraftsPerHour)
		if err != nil {
			return err
		}
		if !ok {
			_ = s.store.AppendAudit(ctx, "rate_limited", "", acc.Name)
			return nil
		}
		token, err := NewApprovalToken()
		if err != nil {
			return err
		}
		if err := s.store.InsertGoalDraft(ctx, g.ID, msgRowID, token, "Reply toward goal: "+g.Objective, body); err != nil {
			return err
		}
		if err := s.store.SetGoalStatus(ctx, g.ID, GoalActive, "counterpart replied; next reply drafted"); err != nil {
			return err
		}
		post := formatGoalPost(g, fmt.Sprintf("They replied:\n%s", firstLine(m.BodyText, 400)), body, token)
		if err := s.publishForAccount(ctx, acc, post, token); err != nil {
			return err
		}
		s.log.Info("goal reply drafted", zap.Int64("goal", g.ID), zap.String("token", token))
		return nil
	}
}

// approveGoalDraft sends an approved goal draft and advances the goal state.
// Openers and follow-ups (anchored to the synthetic goal message) go out as new
// mail threaded onto our own Message-IDs; replies thread onto the inbound mail.
func (s *Service) approveGoalDraft(ctx context.Context, goalID int64, token string, d *StoredDraft, msg *StoredMessage, headers map[string]string) (string, error) {
	g, err := s.store.GetGoal(ctx, goalID)
	if err != nil {
		return "", err
	}
	be := s.backends[g.AccountName]
	if be == nil {
		return "Internal error: no backend for account " + g.AccountName, nil
	}
	wasFollowup := g.Status == GoalWaitingReply

	var sentMsgID string
	if strings.HasPrefix(msg.ProviderMsgID, "goal:") {
		sender, ok := be.(NewMailSender)
		if !ok {
			return "Backend cannot send new mail for this goal.", nil
		}
		inReplyTo := ""
		if n := len(g.ThreadRefs); n > 0 {
			inReplyTo = g.ThreadRefs[n-1]
		}
		references := strings.Join(g.ThreadRefs, " ")
		sentMsgID, err = sender.SendNew(ctx, g.Counterpart, g.Subject, d.Body, inReplyTo, references)
	} else {
		mm := &MailMessage{
			Ref:        Ref{AccountName: msg.AccountName, ProviderID: msg.ProviderMsgID},
			From:       msg.FromAddr,
			Subject:    msg.Subject,
			BodyText:   msg.Snippet,
			MessageID:  headers["Message-ID"],
			InReplyTo:  headers["In-Reply-To"],
			References: headers["References"],
		}
		err = be.Reply(ctx, mm, d.Body)
	}
	if err != nil {
		_ = s.store.AppendAudit(ctx, "send_failed", token, err.Error())
		return "Failed to send goal mail: " + err.Error(), nil
	}
	if sentMsgID != "" {
		_ = s.store.AddGoalThreadRef(ctx, g.ID, sentMsgID)
	}
	_ = s.store.SetDraftStatus(ctx, token, DraftSent)
	_ = s.store.AppendAudit(ctx, "send_ok", token, fmt.Sprintf("goal=%d", g.ID))
	_ = s.store.AppendGoalEvent(ctx, g.ID, "sent", truncateForEvent(d.Body))
	if wasFollowup {
		_ = s.store.IncrementGoalFollowups(ctx, g.ID)
	}
	if err := s.store.SetGoalStatus(ctx, g.ID, GoalWaitingReply, "mail sent; awaiting reply"); err != nil {
		return "", err
	}
	return fmt.Sprintf("Sent goal mail for token %s. Goal #%d is now waiting on %s.", token, g.ID, g.Counterpart), nil
}

// RunGoalFollowups periodically drafts follow-ups for goals whose counterpart
// has gone silent past the follow-up window. Started from Service.Run.
func (s *Service) RunGoalFollowups(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tickGoalFollowups(ctx)
		}
	}
}

func (s *Service) tickGoalFollowups(ctx context.Context) {
	due, err := s.store.GoalsDueFollowup(ctx, time.Now())
	if err != nil {
		s.log.Warn("goal followup scan failed", zap.Error(err))
		return
	}
	for _, g := range due {
		if err := s.draftGoalFollowup(ctx, g); err != nil {
			s.log.Warn("goal followup draft failed", zap.Int64("goal", g.ID), zap.Error(err))
		}
	}
}

func (s *Service) draftGoalFollowup(ctx context.Context, g *Goal) error {
	acc, ok := s.accountByName(g.AccountName)
	if !ok {
		return fmt.Errorf("unknown account %q", g.AccountName)
	}
	transcript, err := s.goalTranscript(ctx, g.ID)
	if err != nil {
		return err
	}
	silentFor := time.Since(g.LastActivity).Round(time.Hour)
	user := fmt.Sprintf("Objective: %s\nCounterpart: %s\nNo reply for: %s\n\nThread so far:\n%s\n\nWrite the follow-up.",
		g.Objective, g.Counterpart, silentFor, transcript)
	body, err := s.runLLM(ctx, goalFollowupSystem, user)
	if err != nil {
		return err
	}
	anchorID, err := s.store.MessageRowID(g.AccountName, goalAnchorProviderID(g.ID))
	if err != nil {
		return err
	}
	token, err := NewApprovalToken()
	if err != nil {
		return err
	}
	if err := s.store.InsertGoalDraft(ctx, g.ID, anchorID, token, fmt.Sprintf("Follow-up %d/%d: %s", g.FollowupsSent+1, g.MaxFollowups, g.Objective), body); err != nil {
		return err
	}
	post := formatGoalPost(g, fmt.Sprintf("No reply for %s — follow-up %d/%d drafted.", silentFor, g.FollowupsSent+1, g.MaxFollowups), body, token)
	if err := s.publishForAccount(ctx, acc, post, token); err != nil {
		return err
	}
	// Reset the silence timer so the next tick doesn't re-draft while this one is pending.
	return s.TouchGoal(ctx, g.ID)
}

// TouchGoal exposes the follow-up timer reset for approval/reject paths.
func (s *Service) TouchGoal(ctx context.Context, id int64) error {
	return s.store.TouchGoal(ctx, id)
}

// goalTranscript renders the sent/received history for LLM context.
func (s *Service) goalTranscript(ctx context.Context, goalID int64) (string, error) {
	events, err := s.store.ListGoalEvents(ctx, goalID, 50)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	for _, e := range events {
		switch e.Kind {
		case "sent":
			fmt.Fprintf(&b, "--- We sent (%s) ---\n%s\n\n", e.CreatedAt.Format("Jan 2 15:04"), e.Detail)
		case "inbound":
			fmt.Fprintf(&b, "--- They wrote (%s) ---\n%s\n\n", e.CreatedAt.Format("Jan 2 15:04"), e.Detail)
		}
	}
	if b.Len() == 0 {
		return "(no mail exchanged yet)", nil
	}
	return b.String(), nil
}

// parseGoalAssessment extracts the STATUS line from a goal LLM response.
// Missing or malformed status defaults to CONTINUE with the full text as body.
func parseGoalAssessment(out string) (verdict, body string) {
	out = strings.TrimSpace(out)
	lines := strings.SplitN(out, "\n", 2)
	first := strings.ToUpper(strings.TrimSpace(lines[0]))
	rest := ""
	if len(lines) > 1 {
		rest = strings.TrimSpace(lines[1])
	}
	switch {
	case strings.HasPrefix(first, "STATUS: ACHIEVED"), strings.HasPrefix(first, "STATUS:ACHIEVED"):
		return "ACHIEVED", rest
	case strings.HasPrefix(first, "STATUS: BLOCKED"), strings.HasPrefix(first, "STATUS:BLOCKED"):
		return "BLOCKED", rest
	case strings.HasPrefix(first, "STATUS: CONTINUE"), strings.HasPrefix(first, "STATUS:CONTINUE"):
		return "CONTINUE", rest
	default:
		return "CONTINUE", out
	}
}

func formatGoalPost(g *Goal, context, draftBody, token string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Goal #%d] %s\n", g.ID, g.Objective)
	fmt.Fprintf(&b, "account: %s | with: %s | subject: %s\n\n", g.AccountName, g.Counterpart, g.Subject)
	fmt.Fprintf(&b, "%s\n\n", strings.TrimSpace(context))
	fmt.Fprintf(&b, "draft:\n%s\n\n", strings.TrimSpace(draftBody))
	fmt.Fprintf(&b, "reply with:  approve %s  |  edit %s: <new body>  |  reject %s\n", token, token, token)
	return b.String()
}
