package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestFormFieldsOrderingArchiveAndOptions(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	campaign := createCampaignForTest(t, q, owner, org, "camp_form", "form", "strict")
	ctx := context.Background()
	for _, input := range []SaveFormFieldInput{
		{PublicID: "field_one", CampaignID: campaign.ID, FieldType: "textarea", Label: "First", ConfigJSON: `{"max_length":100}`},
		{PublicID: "field_two", CampaignID: campaign.ID, FieldType: "radio_group", Label: "Second"},
	} {
		if err := q.CreateFormField(ctx, input, owner.ID); err != nil {
			t.Fatal(err)
		}
	}
	second, err := q.GetFormField(ctx, campaign.ID, "field_two")
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CreateFormOption(ctx, campaign.ID, second.ID, "option_one", "Bugs", "bugs", owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.MoveFormField(ctx, campaign.ID, "field_two", "up", owner.ID); err != nil {
		t.Fatal(err)
	}
	fields, err := q.ListFormFields(ctx, campaign.ID, false)
	if err != nil || len(fields) != 2 || fields[0].PublicID != "field_two" || len(fields[0].Options) != 1 {
		t.Fatalf("unexpected form: %#v err=%v", fields, err)
	}
	if err := q.ArchiveFormField(ctx, campaign.ID, "field_two", owner.ID); err != nil {
		t.Fatal(err)
	}
	fields, _ = q.ListFormFields(ctx, campaign.ID, false)
	if len(fields) != 1 || fields[0].PublicID != "field_one" {
		t.Fatalf("archived field remained active: %#v", fields)
	}
}

func TestSubmissionPrivacyLinkageSnapshotsAndQuota(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	campaign := createCampaignForTest(t, q, owner, org, "camp_submit", "submit", "strict")
	ctx := context.Background()
	if err := q.CreateFormField(ctx, SaveFormFieldInput{PublicID: "field_text", CampaignID: campaign.ID, FieldType: "textarea", Label: "Original"}, owner.ID); err != nil {
		t.Fatal(err)
	}
	field, _ := q.GetFormField(ctx, campaign.ID, "field_text")
	at := time.Date(2026, 6, 12, 12, 0, 0, 0, time.UTC)
	if err := q.RecordCampaignVisit(ctx, RecordVisitInput{
		PublicID: "visit_submit", CampaignID: campaign.ID, OrganizationID: org.ID,
		TokenHash: "one-way-hash", CountRaw: true, CountUnique: true, CollectToken: true, CreatedAt: at,
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal("<script>alert(1)</script>")
	input := CreateSubmissionInput{
		PublicID: "submission_one", VisitPublicID: "visit_submit", CampaignID: campaign.ID, OrgID: org.ID, SubmittedAt: at,
		Answers: []SubmissionAnswerInput{{FieldID: field.ID, FieldPublicID: field.PublicID, FieldType: field.FieldType, FieldLabelSnapshot: field.Label, ValueJSON: string(raw)}},
	}
	if err := q.CreateSubmission(ctx, input); err != nil {
		t.Fatal(err)
	}
	submission, err := q.GetSubmission(ctx, campaign.ID, input.PublicID)
	if err != nil || !submission.VisitPublicID.Valid || submission.VisitPublicID.String != "visit_submit" || !submission.HasInstallTokenHash {
		t.Fatalf("submission linkage failed: %#v err=%v", submission, err)
	}
	if len(submission.Answers) != 1 || submission.Answers[0].FieldLabelSnapshot != "Original" || submission.Answers[0].FieldPublicID != "field_text" {
		t.Fatalf("answer snapshot missing: %#v", submission.Answers)
	}
	var tokenHash string
	if err := q.RawDB().QueryRow(`SELECT install_token_hash FROM campaign_submissions WHERE public_id=?`, input.PublicID).Scan(&tokenHash); err != nil || tokenHash != "one-way-hash" {
		t.Fatalf("hash was not copied from visit: %q %v", tokenHash, err)
	}
	if _, err := q.RawDB().Exec(`UPDATE organization_limits SET max_monthly_submissions=1 WHERE organization_id=?`, org.ID); err != nil {
		t.Fatal(err)
	}
	input.PublicID = "submission_two"
	if err := q.CreateSubmission(ctx, input); !errors.Is(err, ErrSubmissionLimitReached) {
		t.Fatalf("quota not enforced: %v", err)
	}
	stats, err := q.SubmissionStats(ctx, campaign.ID, at)
	if err != nil || stats.Total != 1 || stats.CurrentMonth != 1 || !stats.LatestAt.Valid {
		t.Fatalf("unexpected stats: %#v err=%v", stats, err)
	}
}
