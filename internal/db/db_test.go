package db

import (
	"context"
	"testing"
)

func TestMigrationsAndCoreQueries(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, err := Open(ctx, t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if err := Migrate(database); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	if err := Migrate(database); err != nil {
		t.Fatalf("migrations were not idempotent: %v", err)
	}

	var foreignKeys int
	if err := database.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign key pragma: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign keys are disabled: %d", foreignKeys)
	}

	queries := NewQuerier(database)
	user, organization, err := queries.CreateFirstOwner(ctx, FirstOwnerInput{
		UserPublicID: "usr_test", Username: "Owner", UsernameNormalized: "owner",
		DisplayName: "Owner", PasswordHash: "not-raw", OrganizationPublicID: "org_test",
		OrganizationSlug: "owner", OrganizationName: "Owner organization",
		InstanceName: "Test", InviteOnly: true, AuditAction: "first_setup_owner_created",
		AuditSource: "test",
	})
	if err != nil {
		t.Fatalf("create owner: %v", err)
	}
	if user.ID == 0 || organization.ID == 0 {
		t.Fatal("owner or organization did not receive an ID")
	}
	organizations, err := queries.ListOrganizationsForUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("list organizations: %v", err)
	}
	if len(organizations) != 1 || organizations[0].Role != "owner" {
		t.Fatalf("unexpected organizations: %#v", organizations)
	}
	if err := queries.CreateSession(ctx, user.ID, "hashed-token", Now(), "2999-01-01T00:00:00Z"); err != nil {
		t.Fatalf("create session: %v", err)
	}
	sessionUser, err := queries.GetActiveSessionUser(ctx, "hashed-token", Now())
	if err != nil || sessionUser.ID != user.ID {
		t.Fatalf("read active session: user=%#v error=%v", sessionUser, err)
	}
}
