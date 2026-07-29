package email

import (
	"context"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := OpenStore(t.TempDir())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func TestGoalCRUDAndLifecycle(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	g := &Goal{
		AccountName: "personal",
		Counterpart: "Alice@Example.com",
		Subject:     "Invoice #42",
		Objective:   "Get the invoice corrected",
	}
	id, err := st.InsertGoal(ctx, g)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	got, err := st.GetGoal(ctx, id)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Counterpart != "alice@example.com" {
		t.Errorf("counterpart not normalized: %q", got.Counterpart)
	}
	if got.Status != GoalActive || got.MaxFollowups != 3 || got.FollowupAfter != 48*time.Hour {
		t.Errorf("defaults wrong: %+v", got)
	}

	if err := st.AddGoalThreadRef(ctx, id, "<msg-1@x>"); err != nil {
		t.Fatalf("add ref: %v", err)
	}
	_ = st.AddGoalThreadRef(ctx, id, "<msg-1@x>") // dedupe
	got, _ = st.GetGoal(ctx, id)
	if len(got.ThreadRefs) != 1 {
		t.Errorf("thread refs not deduped: %v", got.ThreadRefs)
	}

	if err := st.SetGoalStatus(ctx, id, GoalAchieved, "confirmed"); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, _ = st.GetGoal(ctx, id)
	if got.Status != GoalAchieved || got.Status.IsOpen() {
		t.Errorf("status: %v open=%v", got.Status, got.Status.IsOpen())
	}

	events, err := st.ListGoalEvents(ctx, id, 10)
	if err != nil || len(events) < 2 {
		t.Fatalf("events: %v err=%v", events, err)
	}
	if events[0].Kind != "created" {
		t.Errorf("first event %q, want created", events[0].Kind)
	}
}

func TestFindActiveGoalForInbound(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	g := &Goal{AccountName: "personal", Counterpart: "alice@example.com", Subject: "s", Objective: "o"}
	id, _ := st.InsertGoal(ctx, g)
	_ = st.AddGoalThreadRef(ctx, id, "<opener@assistclaw>")

	// Match by counterpart address.
	got, err := st.FindActiveGoalForInbound(ctx, "personal", "alice@example.com", nil)
	if err != nil || got == nil || got.ID != id {
		t.Fatalf("by addr: got=%v err=%v", got, err)
	}
	// Match by threading reference even from a different address (forwarded desk).
	got, _ = st.FindActiveGoalForInbound(ctx, "personal", "support@example.com", []string{"<opener@assistclaw>"})
	if got == nil || got.ID != id {
		t.Fatalf("by ref: got=%v", got)
	}
	// Wrong account: no match.
	if got, _ := st.FindActiveGoalForInbound(ctx, "work", "alice@example.com", nil); got != nil {
		t.Fatalf("matched wrong account: %v", got)
	}
	// Closed goals do not route.
	_ = st.SetGoalStatus(ctx, id, GoalAbandoned, "")
	if got, _ := st.FindActiveGoalForInbound(ctx, "personal", "alice@example.com", nil); got != nil {
		t.Fatalf("matched closed goal: %v", got)
	}
}

func TestGoalsDueFollowup(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()

	g := &Goal{AccountName: "a", Counterpart: "c@x.com", Subject: "s", Objective: "o",
		FollowupAfter: time.Hour, MaxFollowups: 2}
	id, _ := st.InsertGoal(ctx, g)
	_ = st.SetGoalStatus(ctx, id, GoalWaitingReply, "")

	// Not due yet.
	due, err := st.GoalsDueFollowup(ctx, time.Now())
	if err != nil || len(due) != 0 {
		t.Fatalf("premature due: %v err=%v", due, err)
	}
	// Due after the window.
	due, _ = st.GoalsDueFollowup(ctx, time.Now().Add(2*time.Hour))
	if len(due) != 1 {
		t.Fatalf("want 1 due, got %v", due)
	}
	// A pending draft suppresses re-drafting.
	anchorID, _ := st.InsertMessage("a", goalAnchorProviderID(id), "c@x.com", "s", "", nil)
	if err := st.InsertGoalDraft(ctx, id, anchorID, "tok1", "sum", "body"); err != nil {
		t.Fatalf("insert goal draft: %v", err)
	}
	due, _ = st.GoalsDueFollowup(ctx, time.Now().Add(2*time.Hour))
	if len(due) != 0 {
		t.Fatalf("pending draft should suppress follow-up: %v", due)
	}
	// Sent draft + exhausted follow-up cap also suppresses.
	_ = st.SetDraftStatus(ctx, "tok1", DraftSent)
	_ = st.IncrementGoalFollowups(ctx, id)
	_ = st.IncrementGoalFollowups(ctx, id)
	due, _ = st.GoalsDueFollowup(ctx, time.Now().Add(2*time.Hour))
	if len(due) != 0 {
		t.Fatalf("cap reached should suppress: %v", due)
	}
}

func TestGoalIDForDraft(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	id, _ := st.InsertGoal(ctx, &Goal{AccountName: "a", Counterpart: "c@x.com", Subject: "s", Objective: "o"})
	msgID, _ := st.InsertMessage("a", goalAnchorProviderID(id), "c@x.com", "s", "", nil)
	_ = st.InsertGoalDraft(ctx, id, msgID, "abcd1234", "sum", "body")

	gid, err := st.GoalIDForDraft(ctx, "ABCD1234")
	if err != nil || gid != id {
		t.Fatalf("goal id for draft: %d err=%v", gid, err)
	}
	// Non-goal drafts return 0.
	m2, _ := st.InsertMessage("a", "uid-9", "x@y.com", "s2", "", nil)
	_ = st.InsertDraft(ctx, m2, "ffff0000", "sum", "body")
	gid, err = st.GoalIDForDraft(ctx, "ffff0000")
	if err != nil || gid != 0 {
		t.Fatalf("plain draft should map to 0: %d err=%v", gid, err)
	}
}

func TestParseGoalAssessment(t *testing.T) {
	cases := []struct {
		in      string
		verdict string
		body    string
	}{
		{"STATUS: ACHIEVED\n\nThey confirmed the refund.", "ACHIEVED", "They confirmed the refund."},
		{"status: blocked\n\nThey want a signed form.", "BLOCKED", "They want a signed form."},
		{"STATUS: CONTINUE\n\nHi Alice,\nThanks.", "CONTINUE", "Hi Alice,\nThanks."},
		{"STATUS:CONTINUE\nbody", "CONTINUE", "body"},
		{"Hi Alice, no status line here.", "CONTINUE", "Hi Alice, no status line here."},
	}
	for _, c := range cases {
		v, b := parseGoalAssessment(c.in)
		if v != c.verdict || b != c.body {
			t.Errorf("parse(%q) = (%q, %q), want (%q, %q)", c.in, v, b, c.verdict, c.body)
		}
	}
}

func TestExtractAddrAndSplitMsgIDs(t *testing.T) {
	if got := extractAddr("Alice Smith <Alice@Example.COM>"); got != "alice@example.com" {
		t.Errorf("extractAddr: %q", got)
	}
	if got := extractAddr("<bob@x.com>"); got != "bob@x.com" {
		t.Errorf("extractAddr bare: %q", got)
	}
	refs := splitMsgIDs("<a@x> <b@y>", "<c@z>")
	if len(refs) != 3 || refs[2] != "<c@z>" {
		t.Errorf("splitMsgIDs: %v", refs)
	}
}
