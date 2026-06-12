package db

import (
	"context"
	"errors"
	"testing"
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
	if !strict.HashInstallToken || strict.CollectReferrerDomain || strict.CollectCoarseBrowser || strict.CollectCoarseOS {
		t.Fatalf("strict defaults are not privacy-strict: %#v", strict)
	}
	balancedCampaign := createCampaignForTest(t, q, owner, org, "camp_balanced", "balanced", "balanced")
	balanced, err := q.GetCampaignSettings(context.Background(), balancedCampaign.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !balanced.HashInstallToken || !balanced.CollectReferrerDomain || !balanced.CollectCoarseBrowser || !balanced.CollectCoarseOS {
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
