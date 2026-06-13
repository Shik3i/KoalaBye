package db

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestRecordCampaignVisitCountsRawAndUnique(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	campaign := createCampaignForTest(t, q, owner, org, "camp_visits", "visits", "strict")
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	first := RecordVisitInput{
		PublicID: "visit_one", CampaignID: campaign.ID, OrganizationID: org.ID,
		TokenHash: "hash-one", CountRaw: true, CountUnique: true, CollectToken: true, CreatedAt: now,
	}
	if err := q.RecordCampaignVisit(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	first.PublicID = "visit_two"
	if err := q.RecordCampaignVisit(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	noToken := first
	noToken.PublicID, noToken.TokenHash = "visit_three", ""
	if err := q.RecordCampaignVisit(context.Background(), noToken); err != nil {
		t.Fatal(err)
	}
	stats, err := q.CampaignVisitStats(context.Background(), campaign.ID, now)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RawTotal != 3 || stats.UniqueTokenTotal != 1 || stats.CurrentMonthTotal != 3 {
		t.Fatalf("unexpected stats: %#v", stats)
	}
	var uniqueFlags []int
	rows, err := q.RawDB().Query(`SELECT counted_as_unique_token_visit FROM campaign_visits ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var flag int
		if err := rows.Scan(&flag); err != nil {
			t.Fatal(err)
		}
		uniqueFlags = append(uniqueFlags, flag)
	}
	if len(uniqueFlags) != 3 || uniqueFlags[0] != 1 || uniqueFlags[1] != 0 || uniqueFlags[2] != 0 {
		t.Fatalf("unexpected unique flags: %v", uniqueFlags)
	}
}

func TestRecordCampaignVisitStoresOnlyStructuredURLContext(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	campaign := createCampaignForTest(t, q, owner, org, "camp_context", "context", "balanced")
	input := RecordVisitInput{
		PublicID: "visit_context", CampaignID: campaign.ID, OrganizationID: org.ID,
		CountRaw: true, CreatedAt: time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
		URLContext: map[string]string{"platform": "chrome", "utm_campaign": "uninstall"},
	}
	if err := q.RecordCampaignVisit(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := q.RawDB().QueryRow(`SELECT context_json FROM campaign_visits WHERE public_id=?`, input.PublicID).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var stored map[string]string
	if err := json.Unmarshal([]byte(raw), &stored); err != nil {
		t.Fatal(err)
	}
	if stored["platform"] != "chrome" || stored["utm_campaign"] != "uninstall" || len(stored) != 2 {
		t.Fatalf("stored context=%v", stored)
	}
}

func TestRecordCampaignVisitRespectsDisabledCountersAndQuota(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	campaign := createCampaignForTest(t, q, owner, org, "camp_quota", "quota", "strict")
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	if _, err := q.RawDB().Exec(`UPDATE organization_limits SET max_monthly_visits=1 WHERE organization_id=?`, org.ID); err != nil {
		t.Fatal(err)
	}
	input := RecordVisitInput{
		PublicID: "visit_unique_only", CampaignID: campaign.ID, OrganizationID: org.ID,
		TokenHash: "hash", CountRaw: false, CountUnique: true, CollectToken: true, CreatedAt: now,
	}
	if err := q.RecordCampaignVisit(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	var raw, unique int
	if err := q.RawDB().QueryRow(`SELECT counted_as_raw_visit,counted_as_unique_token_visit FROM campaign_visits WHERE public_id=?`, input.PublicID).Scan(&raw, &unique); err != nil {
		t.Fatal(err)
	}
	if raw != 0 || unique != 1 {
		t.Fatalf("disabled raw counter stored wrong flags: raw=%d unique=%d", raw, unique)
	}
	input.PublicID = "visit_over"
	if err := q.RecordCampaignVisit(context.Background(), input); !errors.Is(err, ErrVisitLimitReached) {
		t.Fatalf("monthly quota not enforced: %v", err)
	}
}

func TestRecordCampaignVisitStoresNothingWhenCollectionIsDisabled(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	campaign := createCampaignForTest(t, q, owner, org, "camp_disabled_visits", "disabled-visits", "strict")
	if _, err := q.RawDB().Exec(`UPDATE organization_limits SET max_monthly_visits=0 WHERE organization_id=?`, org.ID); err != nil {
		t.Fatal(err)
	}
	input := RecordVisitInput{
		PublicID: "visit_not_stored", CampaignID: campaign.ID, OrganizationID: org.ID,
		TokenHash: "ignored", CollectToken: false, CreatedAt: time.Now().UTC(),
	}
	if err := q.RecordCampaignVisit(context.Background(), input); err != nil {
		t.Fatalf("disabled collection should not consume quota: %v", err)
	}
	var count int
	if err := q.RawDB().QueryRow(`SELECT COUNT(*) FROM campaign_visits WHERE campaign_id=?`, campaign.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("disabled collection stored %d visits", count)
	}
}

func TestRecordFormStartDeduplicatesByVisit(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	campaign := createCampaignForTest(t, q, owner, org, "camp_form_start", "form-start", "strict")
	ctx := context.Background()
	now := time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC)
	if err := q.RecordCampaignVisit(ctx, RecordVisitInput{
		PublicID: "visit_form_start", CampaignID: campaign.ID, OrganizationID: org.ID,
		CountRaw: true, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.RecordFormStart(ctx, campaign.ID, "visit_form_start", now); err != nil {
		t.Fatal(err)
	}
	if err := q.RecordFormStart(ctx, campaign.ID, "visit_form_start", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := q.RawDB().QueryRow(`SELECT COUNT(*) FROM campaign_form_starts WHERE campaign_id=?`, campaign.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("form starts=%d, want 1", count)
	}
}
