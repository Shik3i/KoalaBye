package permissions

import (
	"context"
	"testing"

	"github.com/koalastuff/koalabye/internal/db"
)

func TestIsInstanceOwner(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	database, err := db.Open(ctx, t.TempDir()+"/test.db")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer database.Close()
	if err := db.Migrate(database); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	result, err := database.ExecContext(ctx, `
		INSERT INTO users (public_id, username, username_normalized, display_name, password_hash, created_at, updated_at)
		VALUES ('usr_test', 'owner', 'owner', 'Owner', 'hash', ?, ?)`, db.Now(), db.Now())
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
	userID, _ := result.LastInsertId()
	service := New(db.NewQuerier(database))

	allowed, err := service.IsInstanceOwner(ctx, userID)
	if err != nil {
		t.Fatalf("check role before grant: %v", err)
	}
	if allowed {
		t.Fatal("user without owner role was allowed")
	}
	if _, err := database.ExecContext(ctx, `
		INSERT INTO instance_roles (user_id, role, created_at) VALUES (?, 'instance_owner', ?)`,
		userID, db.Now()); err != nil {
		t.Fatalf("insert role: %v", err)
	}
	allowed, err = service.IsInstanceOwner(ctx, userID)
	if err != nil {
		t.Fatalf("check role after grant: %v", err)
	}
	if !allowed {
		t.Fatal("instance owner was denied")
	}
}
