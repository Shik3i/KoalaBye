package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestResponseTagsAssignmentAndExportFilter(t *testing.T) {
	t.Parallel()
	f := newResponseFixture(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	f.submit(t, "sub_a", "refund please", base)
	f.submit(t, "sub_b", "", base.Add(time.Hour))
	f.submit(t, "sub_c", "love it", base.Add(2*time.Hour))

	bulk := func(action, param string, ids ...string) {
		t.Helper()
		if _, err := f.q.BulkUpdateSubmissions(ctx, f.campaign, f.owner.ID, action, param, BulkTarget{PublicIDs: ids}); err != nil {
			t.Fatalf("%s %q: %v", action, param, err)
		}
	}
	bulk("tag_add", "  Billing   Issue ", "sub_a", "sub_b")
	bulk("tag_add", "billing issue", "sub_c") // same tag, different case/spacing
	bulk("tag_remove", "BILLING ISSUE", "sub_c")
	tags, err := f.q.ListResponseTags(ctx, f.campaign.ID)
	if err != nil || len(tags) != 1 || tags[0].Name != "Billing Issue" || tags[0].Count != 2 {
		t.Fatalf("tags: %#v err=%v", tags, err)
	}
	if got := fmt.Sprint(publicIDs(f.page(t, f.owner, ResponseFilter{Tag: "billing issue"}))); got != "[sub_b sub_a]" {
		t.Fatalf("tag filter: %s", got)
	}
	page := f.page(t, f.owner, ResponseFilter{})
	for _, submission := range page.Submissions {
		wantTag := submission.PublicID != "sub_c"
		if wantTag != (len(submission.Tags) == 1) {
			t.Fatalf("tags on list rows: %s %v", submission.PublicID, submission.Tags)
		}
	}
	if _, err := f.q.BulkUpdateSubmissions(ctx, f.campaign, f.owner.ID, "tag_add", "   ", BulkTarget{PublicIDs: []string{"sub_a"}}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank tag accepted: %v", err)
	}

	// assignment: only members with response access can be assigned
	if _, err := f.q.BulkUpdateSubmissions(ctx, f.campaign, f.owner.ID, "assign", f.other.PublicID, BulkTarget{PublicIDs: []string{"sub_a"}}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("non-member assigned: %v", err)
	}
	if _, err := f.q.RawDB().Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id) VALUES(?,?,'member',?,?)`, f.campaign.OrganizationID, f.other.ID, Now(), f.owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.q.SetCampaignMember(ctx, f.campaign, f.other.PublicID, "analyst", f.owner.ID); err != nil {
		t.Fatal(err)
	}
	bulk("assign", f.other.PublicID, "sub_a")
	if got := fmt.Sprint(publicIDs(f.page(t, f.other, ResponseFilter{Assignee: "me"}))); got != "[sub_a]" {
		t.Fatalf("assigned to me: %s", got)
	}
	if got := fmt.Sprint(publicIDs(f.page(t, f.owner, ResponseFilter{Assignee: f.other.PublicID}))); got != "[sub_a]" {
		t.Fatalf("assignee by public id: %s", got)
	}
	if got := fmt.Sprint(publicIDs(f.page(t, f.owner, ResponseFilter{Assignee: "none"}))); got != "[sub_c sub_b]" {
		t.Fatalf("unassigned: %s", got)
	}
	if counts := f.page(t, f.other, ResponseFilter{}).Counts; counts.AssignedMe != 1 {
		t.Fatalf("assigned-me count: %#v", counts)
	}
	if submission := f.page(t, f.owner, ResponseFilter{Assignee: "me"}); len(submission.Submissions) != 0 {
		t.Fatal("owner must not see other's assignment under 'me'")
	}
	got, _ := f.q.GetSubmission(ctx, f.campaign.ID, "sub_a")
	if got.AssigneeName != "second" || !got.AssigneeUserID.Valid {
		t.Fatalf("detail assignee: %#v", got)
	}
	bulk("unassign", "", "sub_a")

	// export honours filter and carries workflow metadata
	sa, _ := f.q.GetSubmission(ctx, f.campaign.ID, "sub_a")
	_ = f.q.MarkSubmissionRead(ctx, sa.ID, f.owner.ID)
	bulk("star", "", "sub_a")
	filter := ResponseFilter{Tag: "billing issue", Content: "text"}
	exported, err := f.q.ListSubmissionsForExport(ctx, f.campaign.ID, f.owner.ID, &filter)
	if err != nil || len(exported) != 1 || exported[0].PublicID != "sub_a" {
		t.Fatalf("filtered export: %#v err=%v", exported, err)
	}
	if e := exported[0]; !e.Starred || !e.HasText || len(e.Tags) != 1 || len(e.ReadBy) != 1 || e.ReadBy[0] != "Owner" || len(e.Answers) != 2 {
		t.Fatalf("export metadata: %#v", e)
	}
	if all, _ := f.q.ListSubmissionsWithAnswers(ctx, f.campaign.ID); len(all) != 3 {
		t.Fatalf("unfiltered export size %d", len(all))
	}

	// deleting a tag removes it from responses
	if err := f.q.DeleteResponseTag(ctx, f.campaign, tags[0].PublicID, f.owner.ID); err != nil {
		t.Fatal(err)
	}
	if remaining, _ := f.q.ListResponseTags(ctx, f.campaign.ID); len(remaining) != 0 {
		t.Fatalf("tag not deleted: %#v", remaining)
	}
	if err := f.q.DeleteResponseTag(ctx, f.campaign, tags[0].PublicID, f.owner.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("second delete: %v", err)
	}
}

func TestResponseNotesAndSavedViews(t *testing.T) {
	t.Parallel()
	f := newResponseFixture(t)
	ctx := context.Background()
	f.submit(t, "sub_a", "text", time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	sub, _ := f.q.GetSubmission(ctx, f.campaign.ID, "sub_a")

	if err := f.q.AddSubmissionNote(ctx, f.campaign, "sub_a", f.owner.ID, "  looks like a billing bug  "); err != nil {
		t.Fatal(err)
	}
	if err := f.q.AddSubmissionNote(ctx, f.campaign, "sub_a", f.other.ID, "agreed"); err != nil {
		t.Fatal(err)
	}
	for _, body := range []string{"", "   ", strings.Repeat("x", maxNoteRunes+1)} {
		if err := f.q.AddSubmissionNote(ctx, f.campaign, "sub_a", f.owner.ID, body); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("invalid note accepted (%d chars): %v", len(body), err)
		}
	}
	if err := f.q.AddSubmissionNote(ctx, f.campaign, "nope", f.owner.ID, "x"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("note on unknown submission: %v", err)
	}
	notes, err := f.q.ListSubmissionNotes(ctx, sub.ID)
	if err != nil || len(notes) != 2 || notes[0].Body != "looks like a billing bug" || notes[0].AuthorName != "Owner" {
		t.Fatalf("notes: %#v err=%v", notes, err)
	}
	if page := f.page(t, f.owner, ResponseFilter{}); page.Submissions[0].NoteCount != 2 {
		t.Fatalf("note count on list: %d", page.Submissions[0].NoteCount)
	}
	if err := f.q.DeleteSubmissionNote(ctx, f.campaign, notes[0].PublicID, f.other.ID, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("non-author deleted someone else's note: %v", err)
	}
	if err := f.q.DeleteSubmissionNote(ctx, f.campaign, notes[0].PublicID, f.other.ID, true); err != nil {
		t.Fatalf("moderator delete: %v", err)
	}
	if err := f.q.DeleteSubmissionNote(ctx, f.campaign, notes[1].PublicID, f.other.ID, false); err != nil {
		t.Fatalf("author delete: %v", err)
	}

	// saved views
	filter := ResponseFilter{Status: "open", Read: "unread_me", Query: "billing", Page: 4}
	if err := f.q.CreateResponseView(ctx, f.campaign.ID, f.owner.ID, "Billing todo", filter, true); err != nil {
		t.Fatal(err)
	}
	if err := f.q.CreateResponseView(ctx, f.campaign.ID, f.owner.ID, "Private one", ResponseFilter{Starred: true}, false); err != nil {
		t.Fatal(err)
	}
	if err := f.q.CreateResponseView(ctx, f.campaign.ID, f.owner.ID, "empty", ResponseFilter{}, false); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("empty filter saved as view: %v", err)
	}
	if err := f.q.CreateResponseView(ctx, f.campaign.ID, f.owner.ID, " ", filter, false); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("blank view name: %v", err)
	}
	ownerViews, _ := f.q.ListResponseViews(ctx, f.campaign.ID, f.owner.ID)
	otherViews, _ := f.q.ListResponseViews(ctx, f.campaign.ID, f.other.ID)
	if len(ownerViews) != 2 || len(otherViews) != 1 || otherViews[0].Name != "Billing todo" || otherViews[0].IsMine || !ownerViews[0].IsMine {
		t.Fatalf("view visibility: owner=%#v other=%#v", ownerViews, otherViews)
	}
	if got := ownerViews[0].Filter; got.Status != "open" || got.Read != "unread_me" || got.Query != "billing" || got.Page != 1 {
		t.Fatalf("stored filter: %#v", got)
	}
	if err := f.q.DeleteResponseView(ctx, f.campaign.ID, otherViews[0].PublicID, f.other.ID, false); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("non-owner deleted shared view: %v", err)
	}
	if err := f.q.DeleteResponseView(ctx, f.campaign.ID, otherViews[0].PublicID, f.other.ID, true); err != nil {
		t.Fatalf("moderator delete view: %v", err)
	}
	for i := 0; i < maxViewsPerUser; i++ {
		_ = f.q.CreateResponseView(ctx, f.campaign.ID, f.owner.ID, fmt.Sprintf("v%d", i), ResponseFilter{Starred: true}, false)
	}
	if err := f.q.CreateResponseView(ctx, f.campaign.ID, f.owner.ID, "one too many", ResponseFilter{Starred: true}, false); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("view limit: %v", err)
	}
}

func TestAutoCloseOnlyTouchesReadUnassignedUnstarredOldResponses(t *testing.T) {
	t.Parallel()
	f := newResponseFixture(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -40)
	for _, id := range []string{"old_read", "old_unread", "old_starred", "old_assigned", "old_actionable", "new_read"} {
		at := old
		if id == "new_read" {
			at = now.AddDate(0, 0, -2)
		}
		f.submit(t, id, "text", at)
		s, _ := f.q.GetSubmission(ctx, f.campaign.ID, id)
		if id != "old_unread" {
			_ = f.q.MarkSubmissionRead(ctx, s.ID, f.owner.ID)
		}
	}
	if _, err := f.q.RawDB().Exec(`UPDATE campaign_submissions SET starred=1 WHERE public_id='old_starred'`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.RawDB().Exec(`UPDATE campaign_submissions SET assignee_user_id=? WHERE public_id='old_assigned'`, f.owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.q.RawDB().Exec(`UPDATE campaign_submissions SET triage_status='actionable' WHERE public_id='old_actionable'`); err != nil {
		t.Fatal(err)
	}
	if n, err := f.q.RunAutoClose(ctx, now); err != nil || n != 0 {
		t.Fatalf("disabled auto-close touched %d rows (err=%v)", n, err)
	}
	if err := f.q.SetAutoCloseDays(ctx, f.campaign, sql.NullInt64{Int64: 30, Valid: true}, f.owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.q.SetAutoCloseDays(ctx, f.campaign, sql.NullInt64{Int64: 0, Valid: true}, f.owner.ID); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("0 days accepted: %v", err)
	}
	if n, err := f.q.RunAutoClose(ctx, now); err != nil || n != 1 {
		t.Fatalf("auto-close n=%d err=%v", n, err)
	}
	closed := f.page(t, f.owner, ResponseFilter{Status: "closed"})
	if got := fmt.Sprint(publicIDs(closed)); got != "[old_read]" {
		t.Fatalf("closed: %s", got)
	}
	if n, _ := f.q.RunAutoClose(ctx, now); n != 0 {
		t.Fatalf("auto-close not idempotent: %d", n)
	}
	var audits int
	if err := f.q.RawDB().QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='campaign_response_auto_closed' AND actor_user_id IS NULL`).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("auto-close audit: %d err=%v", audits, err)
	}
	settings, _ := f.q.GetCampaignSettings(ctx, f.campaign.ID)
	if !settings.AutoCloseDays.Valid || settings.AutoCloseDays.Int64 != 30 {
		t.Fatalf("settings: %#v", settings.AutoCloseDays)
	}
	if err := f.q.SetAutoCloseDays(ctx, f.campaign, sql.NullInt64{}, f.owner.ID); err != nil {
		t.Fatal(err)
	}
}

func TestComputeTextInsights(t *testing.T) {
	t.Parallel()
	insights := ComputeTextInsights([]string{
		"The sync broke again, sync is broken!",
		"Sync broke for me",
		"Too expensive, too expensive",
		"Zu teuer und der Sync ist kaputt",
		"Too   EXPENSIVE too expensive",
		"ok",
	})
	if insights.Analyzed != 6 {
		t.Fatalf("analyzed=%d", insights.Analyzed)
	}
	words := map[string]int64{}
	for _, w := range insights.TopWords {
		words[w.Word] = w.Count
	}
	if words["sync"] != 3 {
		t.Fatalf("sync counted once per answer: %v", words)
	}
	if words["expensive"] != 2 || words["broke"] != 2 {
		t.Fatalf("word counts: %v", words)
	}
	if _, stop := words["the"]; stop {
		t.Fatalf("stopword leaked: %v", words)
	}
	if insights.TopWords[0].Word != "sync" {
		t.Fatalf("not sorted by count: %v", insights.TopWords)
	}
	if len(insights.Duplicates) != 1 || insights.Duplicates[0].Count != 2 || !strings.HasPrefix(strings.ToLower(insights.Duplicates[0].Sample), "too") {
		t.Fatalf("duplicates: %#v", insights.Duplicates)
	}
}

func TestResponseTextInsightsQuery(t *testing.T) {
	t.Parallel()
	f := newResponseFixture(t)
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	f.submit(t, "a", "sync broke", base)
	f.submit(t, "b", "sync broke", base.Add(time.Minute))
	f.submit(t, "c", "", base.Add(2*time.Minute))
	insights, err := f.q.ResponseTextInsights(context.Background(), f.campaign.ID)
	if err != nil || insights.Analyzed != 2 || len(insights.TopWords) != 2 || len(insights.Duplicates) != 1 {
		t.Fatalf("insights: %#v err=%v", insights, err)
	}
}
