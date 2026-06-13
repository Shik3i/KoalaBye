package db

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"
)

func createCampaignForTest(t *testing.T, q *Querier, owner User, org Organization, publicID, slug, preset string) Campaign {
	t.Helper()
	campaign, err := q.CreateCampaign(context.Background(), CreateCampaignInput{
		PublicID: publicID, OrganizationID: org.ID, CreatedBy: owner.ID, Name: slug,
		Slug: slug, Language: "en", PrivacyPreset: preset,
	})
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := q.GetCampaignByPublicID(context.Background(), org.PublicID, campaign.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func TestCampaignQuotaAndSlugScope(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	ctx := context.Background()
	if _, err := q.RawDB().Exec(`UPDATE organization_limits SET max_campaigns=1 WHERE organization_id=?`, org.ID); err != nil {
		t.Fatal(err)
	}
	createCampaignForTest(t, q, owner, org, "camp_one", "same-slug", "strict")
	if campaigns, err := q.ListCampaignsForUser(ctx, org.ID, owner.ID); err != nil || len(campaigns) != 1 {
		t.Fatalf("list campaign: count=%d err=%v", len(campaigns), err)
	}
	if _, err := q.CreateCampaign(ctx, CreateCampaignInput{PublicID: "camp_over", OrganizationID: org.ID, CreatedBy: owner.ID, Name: "Over", Slug: "other", Language: "en", PrivacyPreset: "strict"}); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("campaign limit not enforced: %v", err)
	}
	limits := DefaultLimits{MaxOrganizationsPerUser: 2, MaxCampaignsPerOrg: 2, MaxMembersPerOrg: 3, MaxActiveInvitesPerOrg: 2, MaxMonthlyVisitsPerOrg: 100, MaxMonthlySubmissionsPerOrg: 10}
	other, err := q.CreateOrganization(ctx, CreateOrganizationInput{PublicID: "org_other", Slug: "other", Name: "Other", UserID: owner.ID, Limits: limits})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = q.CreateCampaign(ctx, CreateCampaignInput{PublicID: "camp_other", OrganizationID: other.ID, CreatedBy: owner.ID, Name: "Same", Slug: "same-slug", Language: "en", PrivacyPreset: "strict"}); err != nil {
		t.Fatalf("same slug in another organization failed: %v", err)
	}
}

func TestCampaignPrivacyPresets(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	strictCampaign := createCampaignForTest(t, q, owner, org, "camp_strict", "strict", "strict")
	if _, err := q.CreateCampaign(context.Background(), CreateCampaignInput{PublicID: "camp_duplicate", OrganizationID: org.ID, CreatedBy: owner.ID, Name: "Duplicate", Slug: "strict", Language: "en", PrivacyPreset: "strict"}); err == nil {
		t.Fatal("duplicate slug in the same organization was accepted")
	}
	strict, err := q.GetCampaignSettings(context.Background(), strictCampaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strict.HashInstallToken || strict.CollectReferrerDomain || strict.CollectCoarseBrowser || strict.CollectCoarseOS || strict.CollectURLContext {
		t.Fatalf("strict defaults are not privacy-strict: %#v", strict)
	}
	balancedCampaign := createCampaignForTest(t, q, owner, org, "camp_balanced", "balanced", "balanced")
	balanced, err := q.GetCampaignSettings(context.Background(), balancedCampaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !balanced.HashInstallToken || !balanced.CollectReferrerDomain || !balanced.CollectCoarseBrowser || !balanced.CollectCoarseOS || !balanced.CollectURLContext {
		t.Fatalf("balanced preset missing coarse fields: %#v", balanced)
	}
}

func TestCampaignAccessAndLastOwner(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	ctx := context.Background()
	campaign := createCampaignForTest(t, q, owner, org, "camp_access", "access", "strict")
	viewer := createTestUser(t, q, "campaignviewer")
	outsider := createTestUser(t, q, "outsider")
	if _, err := q.RawDB().Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id) VALUES(?,?,'member',?,?)`, org.ID, viewer.ID, Now(), owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.SetCampaignMember(ctx, campaign, viewer.PublicID, "viewer", owner.ID); err != nil {
		t.Fatal(err)
	}
	if role, err := q.CampaignRole(ctx, campaign.ID, viewer.ID); err != nil || role != "viewer" {
		t.Fatalf("explicit viewer role=%q err=%v", role, err)
	}
	if err := q.SetCampaignMember(ctx, campaign, outsider.PublicID, "viewer", owner.ID); !errors.Is(err, ErrForbidden) {
		t.Fatalf("outsider received access: %v", err)
	}
	if err := q.RemoveCampaignMember(ctx, campaign, owner.PublicID, owner.ID); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("last owner removal allowed: %v", err)
	}
	admin := createTestUser(t, q, "orgadmin")
	if _, err := q.RawDB().Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id) VALUES(?,?,'admin',?,?)`, org.ID, admin.ID, Now(), owner.ID); err != nil {
		t.Fatal(err)
	}
	if role, err := q.CampaignRole(ctx, campaign.ID, admin.ID); err != nil || role != "owner" {
		t.Fatalf("org admin implicit role=%q err=%v", role, err)
	}
}

func TestArchivedCampaignIsReadOnly(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	ctx := context.Background()
	campaign := createCampaignForTest(t, q, owner, org, "camp_archive", "archive", "strict")
	if err := q.ChangeCampaignStatus(ctx, campaign, "active", owner.ID); err != nil {
		t.Fatal(err)
	}
	campaign.Status = "active"
	if err := q.ChangeCampaignStatus(ctx, campaign, "archived", owner.ID); err != nil {
		t.Fatal(err)
	}
	campaign.Status = "archived"
	campaign.Name = "Changed"
	if err := q.UpdateCampaign(ctx, campaign, owner.ID); !errors.Is(err, ErrCampaignArchived) {
		t.Fatalf("archived campaign was editable: %v", err)
	}
	if err := q.ChangeCampaignStatus(ctx, campaign, "active", owner.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("archived campaign was restored: %v", err)
	}
}

func TestCampaignRedirectCanBeManagedAfterArchive(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	ctx := context.Background()
	source := createCampaignForTest(t, q, owner, org, "camp_redirect_source", "redirect-source", "strict")
	target := createCampaignForTest(t, q, owner, org, "camp_redirect_target", "redirect-target", "strict")
	target.Status = "active"
	target.PublicLinkEnabled = true
	if _, err := q.RawDB().Exec(`UPDATE campaigns SET status='active',public_link_enabled=1 WHERE id=?`, target.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.ChangeCampaignStatus(ctx, source, "archived", owner.ID); err != nil {
		t.Fatal(err)
	}
	source.Status = "archived"
	if err := q.SetCampaignRedirect(ctx, source, target.PublicID, owner.ID); err != nil {
		t.Fatalf("set redirect after archive: %v", err)
	}
	redirect, err := q.GetCampaignRedirect(ctx, source.ID)
	if err != nil || redirect.TargetCampaignPublicID != target.PublicID {
		t.Fatalf("redirect=%#v err=%v", redirect, err)
	}
	if err := q.SetCampaignRedirect(ctx, source, "", owner.ID); err != nil {
		t.Fatalf("remove redirect: %v", err)
	}
	if _, err := q.GetCampaignRedirect(ctx, source.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("removed redirect still exists: %v", err)
	}
}

func TestDuplicateCampaignCopiesConfigurationOnly(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	ctx := context.Background()
	source := createCampaignForTest(t, q, owner, org, "camp_source_copy", "source-copy", "strict")
	settings, _ := q.GetCampaignSettings(ctx, source.ID)
	settings.CollectCoarseBrowser = true
	settings.RetentionEnabled = true
	settings.RetentionDays = sql.NullInt64{Int64: 90, Valid: true}
	if err := q.UpdateCampaignPrivacy(ctx, source, settings, owner.ID); err != nil {
		t.Fatal(err)
	}
	branding := CampaignBranding{
		BrandName: sql.NullString{String: "Acme", Valid: true}, AccentPreset: "purple",
		BackgroundStyle: "theme-dark", ShowKoalabyeBranding: false,
	}
	if err := q.UpdateCampaignBranding(ctx, source, branding, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateFormField(ctx, SaveFormFieldInput{PublicID: "field_copy", CampaignID: source.ID, FieldType: "radio_group", Label: "Reason"}, owner.ID); err != nil {
		t.Fatal(err)
	}
	field, _ := q.GetFormField(ctx, source.ID, "field_copy")
	if err := q.CreateFormOption(ctx, source.ID, field.ID, "option_copy", "Bugs", "bugs", owner.ID); err != nil {
		t.Fatal(err)
	}
	target := createCampaignForTest(t, q, owner, org, "camp_target_copy", "target-copy", "strict")
	if _, err := q.RawDB().Exec(`UPDATE campaigns SET status='active',public_link_enabled=1 WHERE id=?`, target.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.SetCampaignRedirect(ctx, source, target.PublicID, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.RecordCampaignVisit(ctx, RecordVisitInput{PublicID: "visit_copy", CampaignID: source.ID, OrganizationID: org.ID, CountRaw: true, CreatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateSubmission(ctx, CreateSubmissionInput{PublicID: "submission_copy", CampaignID: source.ID, OrgID: org.ID, SubmittedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}

	duplicate, err := q.DuplicateCampaign(ctx, DuplicateCampaignInput{Source: source, Name: "Copy of source", ActorID: owner.ID})
	if err != nil {
		t.Fatal(err)
	}
	if duplicate.Status != "draft" || duplicate.PublicLinkEnabled || duplicate.Slug != "source-copy-copy" {
		t.Fatalf("unexpected duplicate basics: %#v", duplicate)
	}
	duplicateSettings, _ := q.GetCampaignSettings(ctx, duplicate.ID)
	if !duplicateSettings.CollectCoarseBrowser || !duplicateSettings.RetentionEnabled || duplicateSettings.RetentionDays.Int64 != 90 {
		t.Fatalf("settings not copied: %#v", duplicateSettings)
	}
	duplicateBranding, _ := q.GetCampaignBranding(ctx, duplicate.ID)
	if duplicateBranding.BrandName.String != "Acme" || duplicateBranding.AccentPreset != "purple" || duplicateBranding.ShowKoalabyeBranding {
		t.Fatalf("branding not copied: %#v", duplicateBranding)
	}
	fields, _ := q.ListFormFields(ctx, duplicate.ID, false)
	if len(fields) != 1 || fields[0].PublicID == "field_copy" || len(fields[0].Options) != 1 || fields[0].Options[0].PublicID == "option_copy" {
		t.Fatalf("form not safely copied: %#v", fields)
	}
	for table, query := range map[string]string{
		"visits":      `SELECT COUNT(*) FROM campaign_visits WHERE campaign_id=?`,
		"submissions": `SELECT COUNT(*) FROM campaign_submissions WHERE campaign_id=?`,
		"redirects":   `SELECT COUNT(*) FROM campaign_redirects WHERE source_campaign_id=?`,
	} {
		var count int
		if err := q.RawDB().QueryRow(query, duplicate.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s copied: count=%d err=%v", table, count, err)
		}
	}
	var members int
	if err := q.RawDB().QueryRow(`SELECT COUNT(*) FROM campaign_members WHERE campaign_id=?`, duplicate.ID).Scan(&members); err != nil || members != 1 {
		t.Fatalf("unexpected copied members: %d %v", members, err)
	}
}
