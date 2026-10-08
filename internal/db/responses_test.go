package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"
)

type responseFixture struct {
	q        *Querier
	owner    User
	other    User
	campaign Campaign
}

func newResponseFixture(t *testing.T) responseFixture {
	t.Helper()
	q, owner, org := phase3DB(t)
	campaign := createCampaignForTest(t, q, owner, org, "camp_resp", "resp", "strict")
	ctx := context.Background()
	if err := q.CreateFormField(ctx, SaveFormFieldInput{PublicID: "field_text", CampaignID: campaign.ID, FieldType: "textarea", Label: "Why?"}, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateFormField(ctx, SaveFormFieldInput{PublicID: "field_reason", CampaignID: campaign.ID, FieldType: "radio_group", Label: "Reason"}, owner.ID); err != nil {
		t.Fatal(err)
	}
	reason, _ := q.GetFormField(ctx, campaign.ID, "field_reason")
	if err := q.CreateFormOption(ctx, campaign.ID, reason.ID, "option_price", "Too expensive", "price", owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RawDB().Exec(`UPDATE organization_limits SET max_monthly_submissions=500 WHERE organization_id=?`, org.ID); err != nil {
		t.Fatal(err)
	}
	return responseFixture{q: q, owner: owner, other: createTestUser(t, q, "second"), campaign: campaign}
}

func (f responseFixture) submit(t *testing.T, publicID, text string, at time.Time) {
	t.Helper()
	ctx := context.Background()
	textField, _ := f.q.GetFormField(ctx, f.campaign.ID, "field_text")
	reasonField, _ := f.q.GetFormField(ctx, f.campaign.ID, "field_reason")
	reasonJSON, _ := json.Marshal("price")
	answers := []SubmissionAnswerInput{{FieldID: reasonField.ID, FieldPublicID: reasonField.PublicID, FieldType: reasonField.FieldType, FieldLabelSnapshot: reasonField.Label, ValueJSON: string(reasonJSON)}}
	if text != "" {
		textJSON, _ := json.Marshal(text)
		answers = append(answers, SubmissionAnswerInput{FieldID: textField.ID, FieldPublicID: textField.PublicID, FieldType: textField.FieldType, FieldLabelSnapshot: textField.Label, ValueJSON: string(textJSON)})
	}
	if err := f.q.CreateSubmission(ctx, CreateSubmissionInput{PublicID: publicID, CampaignID: f.campaign.ID, OrgID: f.campaign.OrganizationID, SubmittedAt: at, Answers: answers}); err != nil {
		t.Fatal(err)
	}
}

func (f responseFixture) page(t *testing.T, user User, filter ResponseFilter) ResponsePage {
	t.Helper()
	page, err := f.q.ListSubmissionsPage(context.Background(), f.campaign.ID, user.ID, filter)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func publicIDs(page ResponsePage) []string {
	ids := make([]string, 0, len(page.Submissions))
	for _, submission := range page.Submissions {
		ids = append(ids, submission.PublicID)
	}
	return ids
}

func TestResponseReadTrackingAndFilters(t *testing.T) {
	t.Parallel()
	f := newResponseFixture(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	f.submit(t, "sub_a", "Too pricey, I want a refund", base)
	f.submit(t, "sub_b", "", base.Add(24*time.Hour))
	f.submit(t, "sub_c", "100% broken_thing", base.Add(48*time.Hour))
	f.submit(t, "sub_d", "   ", base.Add(72*time.Hour))

	page := f.page(t, f.owner, ResponseFilter{})
	if got := publicIDs(page); fmt.Sprint(got) != "[sub_d sub_c sub_b sub_a]" {
		t.Fatalf("default order: %v", got)
	}
	if page.Counts.Total != 4 || page.Counts.Unread != 4 || page.Counts.UnreadMe != 4 || page.Counts.WithText != 2 {
		t.Fatalf("initial counts: %#v", page.Counts)
	}
	if len(page.Submissions[0].Answers) != 2 || page.Submissions[0].Answers[0].DisplayValueJSON != `"Too expensive"` {
		t.Fatalf("answers/labels not loaded for list: %#v", page.Submissions[0].Answers)
	}
	if page.Submissions[0].HasText || !page.Submissions[1].HasText {
		t.Fatalf("has_text wrong: blank=%v text=%v", page.Submissions[0].HasText, page.Submissions[1].HasText)
	}

	sub, _ := f.q.GetSubmission(ctx, f.campaign.ID, "sub_a")
	if err := f.q.MarkSubmissionRead(ctx, sub.ID, f.owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.q.MarkSubmissionRead(ctx, sub.ID, f.owner.ID); err != nil {
		t.Fatalf("second read must be idempotent: %v", err)
	}
	ownerView := f.page(t, f.owner, ResponseFilter{})
	if ownerView.Counts.Unread != 3 || ownerView.Counts.UnreadMe != 3 {
		t.Fatalf("owner counts after read: %#v", ownerView.Counts)
	}
	otherView := f.page(t, f.other, ResponseFilter{})
	if otherView.Counts.Unread != 3 || otherView.Counts.UnreadMe != 4 {
		t.Fatalf("other counts after owner read: %#v", otherView.Counts)
	}
	for _, submission := range otherView.Submissions {
		if submission.PublicID == "sub_a" && (submission.ReadByMe || submission.ReadByOthers != 1) {
			t.Fatalf("sub_a read flags for other user: %#v", submission)
		}
	}

	cases := []struct {
		name   string
		user   User
		filter ResponseFilter
		want   string
	}{
		{"unread by everyone", f.other, ResponseFilter{Read: "unread"}, "[sub_d sub_c sub_b]"},
		{"team read, seen from other", f.other, ResponseFilter{Read: "team"}, "[sub_a]"},
		{"team read, seen from reader itself", f.owner, ResponseFilter{Read: "team"}, "[]"},
		{"read by me", f.owner, ResponseFilter{Read: "mine"}, "[sub_a]"},
		{"not read by me", f.other, ResponseFilter{Read: "unread_me"}, "[sub_d sub_c sub_b sub_a]"},
		{"free text", f.owner, ResponseFilter{Content: "text"}, "[sub_c sub_a]"},
		{"choices only", f.owner, ResponseFilter{Content: "choices"}, "[sub_d sub_b]"},
		{"search", f.owner, ResponseFilter{Query: "refund"}, "[sub_a]"},
		{"search is AND over terms", f.owner, ResponseFilter{Query: "refund broken"}, "[]"},
		{"search escapes LIKE wildcards", f.owner, ResponseFilter{Query: "100%"}, "[sub_c]"},
		{"search underscore is literal", f.owner, ResponseFilter{Query: "broken_thing"}, "[sub_c]"},
		{"search wildcard-only matches nothing", f.owner, ResponseFilter{Query: "%"}, "[sub_c]"},
		{"search by public id", f.owner, ResponseFilter{Query: "sub_b"}, "[sub_b]"},
		{"date range", f.owner, ResponseFilter{From: "2026-06-02", To: "2026-06-03"}, "[sub_c sub_b]"},
		{"oldest first", f.owner, ResponseFilter{Oldest: true}, "[sub_a sub_b sub_c sub_d]"},
		{"invalid values ignored", f.owner, ResponseFilter{Status: "bogus", Read: "x", Content: "y", From: "nope"}, "[sub_d sub_c sub_b sub_a]"},
	}
	for _, tc := range cases {
		page := f.page(t, tc.user, tc.filter)
		if got := fmt.Sprint(publicIDs(page)); got != tc.want {
			t.Errorf("%s: got %s want %s", tc.name, got, tc.want)
		}
		if int(page.Matching) != len(page.Submissions) {
			t.Errorf("%s: matching=%d rows=%d", tc.name, page.Matching, len(page.Submissions))
		}
	}
}

func TestResponseReadersAndNeighbors(t *testing.T) {
	t.Parallel()
	f := newResponseFixture(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	for i, id := range []string{"sub_a", "sub_b", "sub_c"} {
		f.submit(t, id, "text", base.Add(time.Duration(i)*time.Hour))
	}
	b, _ := f.q.GetSubmission(ctx, f.campaign.ID, "sub_b")
	_ = f.q.MarkSubmissionRead(ctx, b.ID, f.owner.ID)
	_ = f.q.MarkSubmissionRead(ctx, b.ID, f.other.ID)
	readers, err := f.q.ListSubmissionReaders(ctx, b.ID, f.other.ID)
	if err != nil || len(readers) != 2 || !readers[1].IsMe || readers[0].IsMe || readers[0].Name != "Owner" {
		t.Fatalf("readers: %#v err=%v", readers, err)
	}
	neighbors, err := f.q.GetSubmissionNeighbors(ctx, f.campaign.ID, b.ID, f.owner.ID)
	if err != nil || neighbors.NewerPublicID != "sub_c" || neighbors.OlderPublicID != "sub_a" || neighbors.NextUnreadPublicID != "sub_a" {
		t.Fatalf("neighbors: %#v err=%v", neighbors, err)
	}
	a, _ := f.q.GetSubmission(ctx, f.campaign.ID, "sub_a")
	c, _ := f.q.GetSubmission(ctx, f.campaign.ID, "sub_c")
	_ = f.q.MarkSubmissionRead(ctx, a.ID, f.owner.ID)
	neighbors, _ = f.q.GetSubmissionNeighbors(ctx, f.campaign.ID, b.ID, f.owner.ID)
	if neighbors.NextUnreadPublicID != "sub_c" {
		t.Fatalf("next unread should wrap to newer: %#v", neighbors)
	}
	_ = f.q.MarkSubmissionRead(ctx, c.ID, f.owner.ID)
	neighbors, _ = f.q.GetSubmissionNeighbors(ctx, f.campaign.ID, b.ID, f.owner.ID)
	if neighbors.NextUnreadPublicID != "" {
		t.Fatalf("no unread left: %#v", neighbors)
	}
	if count, err := f.q.UnreadSubmissionCount(ctx, f.campaign.ID, f.other.ID); err != nil || count != 2 {
		t.Fatalf("unread count for other user: %d err=%v", count, err)
	}
}

func TestResponseBulkActionsAndPagination(t *testing.T) {
	t.Parallel()
	f := newResponseFixture(t)
	ctx := context.Background()
	base := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	total := ResponsePageSize + 5
	for i := 0; i < total; i++ {
		text := ""
		if i%2 == 0 {
			text = "text"
		}
		f.submit(t, fmt.Sprintf("sub_%03d", i), text, base.Add(time.Duration(i)*time.Minute))
	}
	page := f.page(t, f.owner, ResponseFilter{})
	if page.PageCount != 2 || len(page.Submissions) != ResponsePageSize || page.Matching != int64(total) {
		t.Fatalf("page 1: count=%d rows=%d matching=%d", page.PageCount, len(page.Submissions), page.Matching)
	}
	if page2 := f.page(t, f.owner, ResponseFilter{Page: 2}); len(page2.Submissions) != 5 || page2.Page != 2 {
		t.Fatalf("page 2 rows=%d page=%d", len(page2.Submissions), page2.Page)
	}
	if clamped := f.page(t, f.owner, ResponseFilter{Page: 99}); clamped.Page != 2 || len(clamped.Submissions) != 5 {
		t.Fatalf("out-of-range page not clamped: page=%d rows=%d", clamped.Page, len(clamped.Submissions))
	}

	n, err := f.q.BulkUpdateSubmissions(ctx, f.campaign, f.owner.ID, "read", BulkTarget{PublicIDs: []string{"sub_000", "sub_001", "sub_other_campaign"}})
	if err != nil || n != 2 {
		t.Fatalf("bulk read by ids: n=%d err=%v", n, err)
	}
	filter := ResponseFilter{Content: "text"}
	if n, err = f.q.BulkUpdateSubmissions(ctx, f.campaign, f.owner.ID, "read", BulkTarget{Filter: &filter}); err != nil || n != 27 {
		t.Fatalf("bulk read by filter: n=%d err=%v", n, err)
	}
	if counts := f.page(t, f.owner, ResponseFilter{}).Counts; counts.UnreadMe != int64(total-29) {
		t.Fatalf("unread after bulk read: %#v", counts)
	}
	if counts := f.page(t, f.other, ResponseFilter{}).Counts; counts.UnreadMe != int64(total) {
		t.Fatalf("bulk read must not touch other users: %#v", counts)
	}
	if n, err = f.q.BulkUpdateSubmissions(ctx, f.campaign, f.owner.ID, "unread", BulkTarget{PublicIDs: []string{"sub_000"}}); err != nil || n != 1 {
		t.Fatalf("bulk unread: n=%d err=%v", n, err)
	}

	if _, err = f.q.BulkUpdateSubmissions(ctx, f.campaign, f.owner.ID, "star", BulkTarget{PublicIDs: []string{"sub_002", "sub_003"}}); err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(publicIDs(f.page(t, f.owner, ResponseFilter{Starred: true}))); got != "[sub_003 sub_002]" {
		t.Fatalf("starred: %s", got)
	}
	if _, err = f.q.BulkUpdateSubmissions(ctx, f.campaign, f.owner.ID, "status_actionable", BulkTarget{Filter: &ResponseFilter{Starred: true}}); err != nil {
		t.Fatal(err)
	}
	if counts := f.page(t, f.owner, ResponseFilter{}).Counts; counts.Actionable != 2 || counts.Starred != 2 {
		t.Fatalf("status/star counts: %#v", counts)
	}
	if got := f.page(t, f.owner, ResponseFilter{Status: "open"}).Matching; got != int64(total) {
		t.Fatalf("open filter matching=%d", got)
	}
	var audits int
	if err := f.q.RawDB().QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='campaign_response_bulk_updated'`).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("shared-state bulk actions must be audited (once per action), got %d err=%v", audits, err)
	}

	for _, bad := range []struct {
		action string
		target BulkTarget
	}{
		{"drop_table", BulkTarget{PublicIDs: []string{"sub_000"}}},
		{"status_bogus", BulkTarget{PublicIDs: []string{"sub_000"}}},
		{"read", BulkTarget{}},
		{"read", BulkTarget{PublicIDs: make([]string, maxBulkSelectedIDs+1)}},
	} {
		if _, err := f.q.BulkUpdateSubmissions(ctx, f.campaign, f.owner.ID, bad.action, bad.target); !errors.Is(err, ErrForbidden) {
			t.Errorf("action %q target %d ids: expected ErrForbidden, got %v", bad.action, len(bad.target.PublicIDs), err)
		}
	}
}

func TestResponseFilterValuesRoundTrip(t *testing.T) {
	t.Parallel()
	filter := ResponseFilter{Status: "open", Read: "team", Content: "text", Starred: true, Query: "a  b", From: "2026-01-02", To: "2026-02-03", Oldest: true, Page: 3}.Normalize()
	values := filter.Values()
	if values.Get("q") != "a b" || values.Get("page") != "3" || values.Get("sort") != "oldest" || values.Get("starred") != "1" {
		t.Fatalf("unexpected encoding: %v", values)
	}
	if (ResponseFilter{}).Normalize().Values().Encode() != "" || (ResponseFilter{}).IsFiltered() {
		t.Fatal("empty filter must encode to nothing")
	}
}
