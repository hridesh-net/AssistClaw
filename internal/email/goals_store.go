package email

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// InsertGoal persists a new goal and its "created" event; returns the goal ID.
func (s *Store) InsertGoal(ctx context.Context, g *Goal) (int64, error) {
	if g.Status == "" {
		g.Status = GoalActive
	}
	if g.FollowupAfter <= 0 {
		g.FollowupAfter = 48 * time.Hour
	}
	if g.MaxFollowups <= 0 {
		g.MaxFollowups = 3
	}
	refs, _ := json.Marshal(g.ThreadRefs)
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `
		INSERT INTO email_goals(account_name, counterpart, subject, objective, status,
			followup_after_secs, max_followups, followups_sent, thread_refs, last_activity, created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		g.AccountName, strings.ToLower(strings.TrimSpace(g.Counterpart)), g.Subject, g.Objective, string(g.Status),
		int64(g.FollowupAfter.Seconds()), g.MaxFollowups, g.FollowupsSent, string(refs), now, now)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	g.ID = id
	_ = s.AppendGoalEvent(ctx, id, "created", g.Objective)
	return id, nil
}

func (s *Store) scanGoal(row interface{ Scan(...any) error }) (*Goal, error) {
	var g Goal
	var st, refs string
	var fSecs, last, created int64
	err := row.Scan(&g.ID, &g.AccountName, &g.Counterpart, &g.Subject, &g.Objective, &st,
		&fSecs, &g.MaxFollowups, &g.FollowupsSent, &refs, &last, &created)
	if err != nil {
		return nil, err
	}
	g.Status = GoalStatus(st)
	g.FollowupAfter = time.Duration(fSecs) * time.Second
	g.LastActivity = time.Unix(last, 0)
	g.CreatedAt = time.Unix(created, 0)
	_ = json.Unmarshal([]byte(refs), &g.ThreadRefs)
	return &g, nil
}

const goalCols = `id, account_name, counterpart, subject, objective, status,
	followup_after_secs, max_followups, followups_sent, thread_refs, last_activity, created_at`

// GetGoal loads one goal by ID.
func (s *Store) GetGoal(ctx context.Context, id int64) (*Goal, error) {
	return s.scanGoal(s.db.QueryRowContext(ctx,
		`SELECT `+goalCols+` FROM email_goals WHERE id = ?`, id))
}

// ListGoals returns goals, newest first. When openOnly is true, closed goals are skipped.
func (s *Store) ListGoals(ctx context.Context, openOnly bool) ([]*Goal, error) {
	q := `SELECT ` + goalCols + ` FROM email_goals`
	if openOnly {
		q += ` WHERE status IN ('active','waiting_reply','blocked')`
	}
	q += ` ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Goal
	for rows.Next() {
		g, err := s.scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// SetGoalStatus updates status, touches last_activity, and records an event.
func (s *Store) SetGoalStatus(ctx context.Context, id int64, st GoalStatus, detail string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE email_goals SET status = ?, last_activity = ? WHERE id = ?`,
		string(st), time.Now().Unix(), id)
	if err != nil {
		return err
	}
	return s.AppendGoalEvent(ctx, id, string(st), detail)
}

// TouchGoal bumps last_activity (resets the follow-up timer).
func (s *Store) TouchGoal(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE email_goals SET last_activity = ? WHERE id = ?`, time.Now().Unix(), id)
	return err
}

// IncrementGoalFollowups bumps the follow-up counter.
func (s *Store) IncrementGoalFollowups(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE email_goals SET followups_sent = followups_sent + 1 WHERE id = ?`, id)
	return err
}

// AddGoalThreadRef appends a Message-ID to the goal's thread reference set (deduped).
func (s *Store) AddGoalThreadRef(ctx context.Context, id int64, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil
	}
	g, err := s.GetGoal(ctx, id)
	if err != nil {
		return err
	}
	for _, r := range g.ThreadRefs {
		if r == ref {
			return nil
		}
	}
	refs, _ := json.Marshal(append(g.ThreadRefs, ref))
	_, err = s.db.ExecContext(ctx,
		`UPDATE email_goals SET thread_refs = ? WHERE id = ?`, string(refs), id)
	return err
}

// AppendGoalEvent records one transcript/audit entry for a goal.
func (s *Store) AppendGoalEvent(ctx context.Context, goalID int64, kind, detail string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO goal_events(goal_id, created_at, kind, detail) VALUES(?,?,?,?)`,
		goalID, time.Now().Unix(), kind, detail)
	return err
}

// ListGoalEvents returns a goal's events in chronological order.
func (s *Store) ListGoalEvents(ctx context.Context, goalID int64, limit int) ([]GoalEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, goal_id, created_at, kind, detail FROM goal_events
		WHERE goal_id = ? ORDER BY id ASC LIMIT ?`, goalID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GoalEvent
	for rows.Next() {
		var e GoalEvent
		var ts int64
		var detail sql.NullString
		if err := rows.Scan(&e.ID, &e.GoalID, &ts, &e.Kind, &detail); err != nil {
			return nil, err
		}
		e.CreatedAt = time.Unix(ts, 0)
		e.Detail = detail.String
		out = append(out, e)
	}
	return out, rows.Err()
}

// FindActiveGoalForInbound matches an inbound mail to an open goal on the account:
// by sender address equal to the counterpart, or by threading headers referencing
// a Message-ID we have seen or sent on the goal's thread.
func (s *Store) FindActiveGoalForInbound(ctx context.Context, account, fromAddr string, refs []string) (*Goal, error) {
	goals, err := s.ListGoals(ctx, true)
	if err != nil {
		return nil, err
	}
	fromAddr = strings.ToLower(strings.TrimSpace(fromAddr))
	refSet := map[string]bool{}
	for _, r := range refs {
		if r = strings.TrimSpace(r); r != "" {
			refSet[r] = true
		}
	}
	for _, g := range goals {
		if g.AccountName != account {
			continue
		}
		for _, r := range g.ThreadRefs {
			if refSet[r] {
				return g, nil
			}
		}
		if fromAddr != "" && g.Counterpart != "" && strings.Contains(fromAddr, g.Counterpart) {
			return g, nil
		}
	}
	return nil, nil
}

// GoalsDueFollowup returns goals waiting on a reply past their follow-up window,
// under their follow-up cap, with no draft currently pending approval.
func (s *Store) GoalsDueFollowup(ctx context.Context, now time.Time) ([]*Goal, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT `+goalCols+` FROM email_goals g
		WHERE g.status = 'waiting_reply'
		  AND g.followups_sent < g.max_followups
		  AND g.last_activity + g.followup_after_secs <= ?
		  AND NOT EXISTS (
			SELECT 1 FROM email_drafts d WHERE d.goal_id = g.id AND d.status = 'pending'
		  )`, now.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Goal
	for rows.Next() {
		g, err := s.scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// InsertGoalDraft creates a pending draft linked to a goal.
func (s *Store) InsertGoalDraft(ctx context.Context, goalID, messageID int64, token, summary, body string) error {
	ts := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO email_drafts(message_id, token, summary, body, status, created_at, goal_id)
		VALUES(?,?,?,?,?,?,?)`,
		messageID, token, summary, body, string(DraftPending), ts, goalID)
	if err != nil {
		return err
	}
	return s.AppendGoalEvent(ctx, goalID, "draft_created", fmt.Sprintf("token=%s\n%s", token, body))
}

// MessageRowID returns the email_messages row id for a provider message id.
func (s *Store) MessageRowID(account, providerMsgID string) (int64, error) {
	var id int64
	err := s.db.QueryRowContext(context.Background(),
		`SELECT id FROM email_messages WHERE account_name = ? AND provider_msg_id = ?`,
		account, providerMsgID).Scan(&id)
	return id, err
}

// GoalIDForDraft returns the goal a draft belongs to (0 when none).
func (s *Store) GoalIDForDraft(ctx context.Context, token string) (int64, error) {
	var id sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT goal_id FROM email_drafts WHERE lower(token) = ?`,
		strings.TrimSpace(strings.ToLower(token))).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return id.Int64, nil
}
