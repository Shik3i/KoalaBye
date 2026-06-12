package db

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestCampaignAnalyticsTrendsFieldsAndMetadata(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	campaign := createCampaignForTest(t, q, owner, org, "camp_analytics", "analytics", "strict")
	ctx := context.Background()
	fields := []SaveFormFieldInput{
		{PublicID: "field_rating", CampaignID: campaign.ID, FieldType: "rating_1_5", Label: "Rating"},
		{PublicID: "field_radio", CampaignID: campaign.ID, FieldType: "radio_group", Label: "Reason"},
		{PublicID: "field_check", CampaignID: campaign.ID, FieldType: "checkbox_group", Label: "Features"},
	}
	for _, field := range fields {
		if err := q.CreateFormField(ctx, field, owner.ID); err != nil {
			t.Fatal(err)
		}
	}
	rating, _ := q.GetFormField(ctx, campaign.ID, "field_rating")
	radio, _ := q.GetFormField(ctx, campaign.ID, "field_radio")
	check, _ := q.GetFormField(ctx, campaign.ID, "field_check")
	if err := q.CreateFormOption(ctx, campaign.ID, radio.ID, "option_bug", "Bugs", "bugs", owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateFormOption(ctx, campaign.ID, check.ID, "option_speed", "Speed", "speed", owner.ID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -20)
	for index, at := range []time.Time{old, now.Add(-24 * time.Hour), now} {
		if err := q.RecordCampaignVisit(ctx, RecordVisitInput{
			PublicID: "visit_analytics_" + string(rune('a'+index)), CampaignID: campaign.ID, OrganizationID: org.ID,
			TokenHash: "hash-" + string(rune('a'+index)), ReferrerDomain: "example.com", CoarseBrowser: "Firefox", CoarseOS: "Linux",
			CountRaw: true, CountUnique: true, CollectToken: true, CreatedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	makeAnswer := func(field FormField, value any) SubmissionAnswerInput {
		raw, _ := json.Marshal(value)
		return SubmissionAnswerInput{FieldID: field.ID, FieldPublicID: field.PublicID, FieldType: field.FieldType, FieldLabelSnapshot: field.Label, ValueJSON: string(raw)}
	}
	for index, input := range []CreateSubmissionInput{
		{PublicID: "submission_old", CampaignID: campaign.ID, OrgID: org.ID, SubmittedAt: old, Answers: []SubmissionAnswerInput{makeAnswer(rating, 1)}},
		{PublicID: "submission_recent_one", CampaignID: campaign.ID, OrgID: org.ID, SubmittedAt: now.Add(-24 * time.Hour), Answers: []SubmissionAnswerInput{makeAnswer(rating, 4), makeAnswer(radio, "bugs"), makeAnswer(check, []string{"speed"})}},
		{PublicID: "submission_recent_two", CampaignID: campaign.ID, OrgID: org.ID, SubmittedAt: now, Answers: []SubmissionAnswerInput{makeAnswer(rating, 2), makeAnswer(radio, "bugs"), makeAnswer(check, []string{"speed"})}},
	} {
		_ = index
		if err := q.CreateSubmission(ctx, input); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.ArchiveFormField(ctx, campaign.ID, radio.PublicID, owner.ID); err != nil {
		t.Fatal(err)
	}
	start := now.AddDate(0, 0, -6)
	analytics, err := q.CampaignAnalytics(ctx, campaign.ID, &start, now)
	if err != nil {
		t.Fatal(err)
	}
	if analytics.Overview.RawVisits != 3 || analytics.Overview.Submissions != 3 || len(analytics.Trend) != 7 || analytics.Trend[5].RawVisits != 1 || analytics.Trend[6].Submissions != 1 {
		t.Fatalf("overview/trend incorrect: %#v", analytics)
	}
	summaries := map[string]FieldSummary{}
	for _, field := range analytics.Fields {
		summaries[field.PublicID] = field
	}
	if summaries["field_rating"].Answered != 2 || summaries["field_rating"].Average != 3 {
		t.Fatalf("rating summary incorrect: %#v", summaries["field_rating"])
	}
	if !summaries["field_radio"].Archived || summaries["field_radio"].Values[0].Count != 2 {
		t.Fatalf("archived radio summary incorrect: %#v", summaries["field_radio"])
	}
	if summaries["field_check"].TotalSelections != 2 || len(analytics.Referrers) != 1 || analytics.Browsers[0].Value != "Firefox" {
		t.Fatalf("checkbox or metadata summary incorrect: %#v", analytics)
	}
}

func TestRetentionAndManualDeletion(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	campaign := createCampaignForTest(t, q, owner, org, "camp_delete", "delete", "strict")
	ctx := context.Background()
	now := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -100)
	for _, item := range []struct {
		visit, submission string
		at                time.Time
	}{{"visit_old", "submission_old", old}, {"visit_new", "submission_new", now}} {
		if err := q.RecordCampaignVisit(ctx, RecordVisitInput{PublicID: item.visit, CampaignID: campaign.ID, OrganizationID: org.ID, CountRaw: true, CreatedAt: item.at}); err != nil {
			t.Fatal(err)
		}
		if err := q.CreateSubmission(ctx, CreateSubmissionInput{PublicID: item.submission, VisitPublicID: item.visit, CampaignID: campaign.ID, OrgID: org.ID, SubmittedAt: item.at}); err != nil {
			t.Fatal(err)
		}
	}
	counts, err := q.DeleteOldCampaignData(ctx, campaign, now.AddDate(0, 0, -90), owner.ID)
	if err != nil || counts.Visits != 1 || counts.Submissions != 1 {
		t.Fatalf("delete old failed: %#v %v", counts, err)
	}
	if count, err := q.DeleteAllCampaignVisits(ctx, campaign, owner.ID); err != nil || count != 1 {
		t.Fatalf("delete visits failed: %d %v", count, err)
	}
	var linked any
	if err := q.RawDB().QueryRow(`SELECT visit_id FROM campaign_submissions WHERE public_id='submission_new'`).Scan(&linked); err != nil || linked != nil {
		t.Fatalf("remaining submission was not unlinked: %v %v", linked, err)
	}
	if count, err := q.DeleteAllCampaignResponses(ctx, campaign, owner.ID); err != nil || count != 1 {
		t.Fatalf("delete responses failed: %d %v", count, err)
	}
	var audits int
	if err := q.RawDB().QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action IN ('campaign.retention.delete_old','campaign.visits.delete_all','campaign.responses.delete_all')`).Scan(&audits); err != nil || audits != 3 {
		t.Fatalf("deletions not audited: %d %v", audits, err)
	}
}
