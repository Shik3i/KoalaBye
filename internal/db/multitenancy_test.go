package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func phase3DB(t *testing.T) (*Querier, User, Organization) {
	t.Helper()
	ctx := context.Background()
	database, err := Open(ctx, t.TempDir()+"/phase3.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	if err := Migrate(database); err != nil {
		t.Fatal(err)
	}
	q := NewQuerier(database)
	user, org, err := q.CreateFirstOwner(ctx, FirstOwnerInput{
		UserPublicID: "usr_owner", Username: "owner", UsernameNormalized: "owner", DisplayName: "Owner", PasswordHash: "hash",
		OrganizationPublicID: "org_default", OrganizationSlug: "default", OrganizationName: "Default", InstanceName: "Test",
		InviteRegistrationEnabled: true, InviteOnly: true, Limits: DefaultLimits{MaxOrganizationsPerUser: 2, MaxCampaignsPerOrg: 3, MaxMembersPerOrg: 3, MaxActiveInvitesPerOrg: 2, MaxMonthlyVisitsPerOrg: 100, MaxMonthlySubmissionsPerOrg: 10},
		AuditAction: "first_setup_owner_created", AuditSource: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	return q, user, org
}

func createTestUser(t *testing.T, q *Querier, name string) User {
	t.Helper()
	u, err := q.CreateUser(context.Background(), "usr_"+name, name, name, "", "", name, "hash")
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestOrganizationCreationLimit(t *testing.T) {
	t.Parallel()
	q, user, _ := phase3DB(t)
	ctx := context.Background()
	limits := DefaultLimits{MaxOrganizationsPerUser: 2, MaxCampaignsPerOrg: 3, MaxMembersPerOrg: 3, MaxActiveInvitesPerOrg: 2, MaxMonthlyVisitsPerOrg: 100, MaxMonthlySubmissionsPerOrg: 10}
	if _, err := q.CreateOrganization(ctx, CreateOrganizationInput{PublicID: "org_second", Slug: "second", Name: "Second", UserID: user.ID, Limits: limits}); err != nil {
		t.Fatalf("create within limit: %v", err)
	}
	if _, err := q.CreateOrganization(ctx, CreateOrganizationInput{PublicID: "org_third", Slug: "third", Name: "Third", UserID: user.ID, Limits: limits}); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("expected limit, got %v", err)
	}
}

func TestInviteLifecycleAndHashedStorage(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	ctx := context.Background()
	member := createTestUser(t, q, "member")
	raw := "very-secret-invite-code"
	expires := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	if err := q.CreateInvite(ctx, CreateInviteInput{PublicID: "inv_one", CodeHash: HashInviteCode(raw), Role: "member", ExpiresAt: expires, OrganizationID: org.ID, CreatedBy: owner.ID, MaxUses: 1}); err != nil {
		t.Fatal(err)
	}
	var stored string
	if err := q.RawDB().QueryRow(`SELECT code_hash FROM invites WHERE public_id='inv_one'`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == raw || stored != HashInviteCode(raw) {
		t.Fatal("raw invite was stored")
	}
	if err := q.AcceptInvite(ctx, raw, member.ID); err != nil {
		t.Fatalf("accept invite: %v", err)
	}
	if err := q.AcceptInvite(ctx, raw, createTestUser(t, q, "second").ID); !errors.Is(err, ErrInviteUnavailable) {
		t.Fatalf("max uses not enforced: %v", err)
	}
}

func TestActiveInviteLimit(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	for index, code := range []string{"one", "two"} {
		err := q.CreateInvite(context.Background(), CreateInviteInput{
			PublicID: fmt.Sprintf("inv_%d", index), CodeHash: HashInviteCode(code), Role: "viewer",
			ExpiresAt:      time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
			OrganizationID: org.ID, CreatedBy: owner.ID, MaxUses: 1,
		})
		if err != nil {
			t.Fatalf("create invite %d: %v", index, err)
		}
	}
	err := q.CreateInvite(context.Background(), CreateInviteInput{
		PublicID: "inv_over", CodeHash: HashInviteCode("over"), Role: "viewer",
		ExpiresAt:      time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano),
		OrganizationID: org.ID, CreatedBy: owner.ID, MaxUses: 1,
	})
	if !errors.Is(err, ErrLimitReached) {
		t.Fatalf("active invite limit not enforced: %v", err)
	}
}

func TestExpiredAndRevokedInvitesCannotBeAccepted(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		expired bool
		revoked bool
	}{{"expired", true, false}, {"revoked", false, true}}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			q, owner, org := phase3DB(t)
			ctx := context.Background()
			user := createTestUser(t, q, "joiner")
			expiry := time.Now().UTC().Add(time.Hour)
			if tc.expired {
				expiry = time.Now().UTC().Add(-time.Hour)
			}
			if err := q.CreateInvite(ctx, CreateInviteInput{PublicID: "inv_test", CodeHash: HashInviteCode("code"), Role: "viewer", ExpiresAt: expiry.Format(time.RFC3339Nano), OrganizationID: org.ID, CreatedBy: owner.ID, MaxUses: 1}); err != nil {
				t.Fatal(err)
			}
			if tc.revoked {
				if err := q.RevokeInvite(ctx, "inv_test", org.ID, owner.ID); err != nil {
					t.Fatal(err)
				}
			}
			if err := q.AcceptInvite(ctx, "code", user.ID); !errors.Is(err, ErrInviteUnavailable) {
				t.Fatalf("expected unavailable, got %v", err)
			}
		})
	}
}

func TestMemberLimitAndAlreadyMember(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	ctx := context.Background()
	if _, err := q.RawDB().Exec(`UPDATE organization_limits SET max_members=2 WHERE organization_id=?`, org.ID); err != nil {
		t.Fatal(err)
	}
	user := createTestUser(t, q, "one")
	if err := q.CreateInvite(ctx, CreateInviteInput{PublicID: "inv_a", CodeHash: HashInviteCode("a"), Role: "member", ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), OrganizationID: org.ID, CreatedBy: owner.ID, MaxUses: 5}); err != nil {
		t.Fatal(err)
	}
	if err := q.AcceptInvite(ctx, "a", user.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.AcceptInvite(ctx, "a", user.ID); !errors.Is(err, ErrAlreadyMember) {
		t.Fatalf("already member not safe: %v", err)
	}
	if err := q.AcceptInvite(ctx, "a", createTestUser(t, q, "two").ID); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("member limit not enforced: %v", err)
	}
}

func TestOrganizationMustRetainOwner(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	ctx := context.Background()
	if err := q.RemoveMember(ctx, org.ID, owner.ID, owner.ID, "owner", false); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("last owner removal allowed: %v", err)
	}
	if err := q.UpdateMemberRole(ctx, org.ID, owner.ID, owner.ID, "owner", "admin", false); !errors.Is(err, ErrLastOwner) {
		t.Fatalf("last owner demotion allowed: %v", err)
	}
}

func TestAdminCannotManageOwnersButOwnerCanPromote(t *testing.T) {
	t.Parallel()
	q, owner, org := phase3DB(t)
	ctx := context.Background()
	admin := createTestUser(t, q, "admin")
	member := createTestUser(t, q, "member")
	if _, err := q.RawDB().Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id)VALUES(?,?,'admin',?,?)`, org.ID, admin.ID, Now(), owner.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.RawDB().Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id)VALUES(?,?,'member',?,?)`, org.ID, member.ID, Now(), owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateMemberRole(ctx, org.ID, owner.ID, admin.ID, "admin", "member", false); !errors.Is(err, ErrForbidden) {
		t.Fatalf("admin changed owner: %v", err)
	}
	if err := q.UpdateMemberRole(ctx, org.ID, member.ID, owner.ID, "owner", "owner", false); err != nil {
		t.Fatalf("owner could not promote: %v", err)
	}
}
