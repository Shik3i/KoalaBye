package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/koalastuff/koalabye/internal/db/dbgen"
)

type User struct {
	ID                 int64
	PublicID           string
	Username           string
	UsernameNormalized string
	DisplayName        string
	PasswordHash       string
	DisabledAt         sql.NullString
}

type Organization struct {
	ID       int64
	PublicID string
	Slug     string
	Name     string
	Role     string
}

type AuditEvent struct {
	ID             int64
	ActorUserID    sql.NullInt64
	OrganizationID sql.NullInt64
	Action         string
	TargetType     sql.NullString
	TargetID       sql.NullString
	Reason         sql.NullString
	MetadataJSON   sql.NullString
	CreatedAt      string
}

type Querier struct {
	db        *sql.DB
	generated *dbgen.Queries
}

func NewQuerier(database *sql.DB) *Querier {
	return &Querier{db: database, generated: dbgen.New(database)}
}

func (q *Querier) CountInstanceOwners(ctx context.Context) (int64, error) {
	return q.generated.CountInstanceOwners(ctx)
}

func (q *Querier) GetUserByNormalizedUsername(ctx context.Context, username string) (User, error) {
	row, err := q.generated.GetUserByNormalizedUsername(ctx, username)
	if err != nil {
		return User{}, err
	}
	return User{
		ID: row.ID, PublicID: row.PublicID, Username: row.Username,
		UsernameNormalized: row.UsernameNormalized, DisplayName: row.DisplayName,
		PasswordHash: row.PasswordHash, DisabledAt: nullString(row.DisabledAt),
	}, nil
}

func (q *Querier) GetUserByID(ctx context.Context, id int64) (User, error) {
	return scanUser(q.db.QueryRowContext(ctx, `
		SELECT id, public_id, username, username_normalized, display_name, password_hash, disabled_at
		FROM users WHERE id = ? LIMIT 1`, id))
}

type scanner interface {
	Scan(...any) error
}

func scanUser(row scanner) (User, error) {
	var user User
	err := row.Scan(&user.ID, &user.PublicID, &user.Username, &user.UsernameNormalized, &user.DisplayName, &user.PasswordHash, &user.DisabledAt)
	return user, err
}

func (q *Querier) UserHasInstanceRole(ctx context.Context, userID int64, role string) (bool, error) {
	allowed, err := q.generated.UserHasInstanceRole(ctx, dbgen.UserHasInstanceRoleParams{UserID: userID, Role: role})
	return allowed == 1, err
}

func (q *Querier) ListOrganizationsForUser(ctx context.Context, userID int64) ([]Organization, error) {
	rows, err := q.generated.ListOrganizationsForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	organizations := make([]Organization, 0, len(rows))
	for _, row := range rows {
		organizations = append(organizations, Organization{
			ID: row.ID, PublicID: row.PublicID, Slug: row.Slug, Name: row.Name, Role: row.Role,
		})
	}
	return organizations, nil
}

func (q *Querier) UserCanAccessOrganization(ctx context.Context, userID, organizationID int64) (bool, error) {
	allowed, err := q.generated.UserCanAccessOrganization(ctx, dbgen.UserCanAccessOrganizationParams{
		UserID: userID, OrganizationID: organizationID,
	})
	return allowed == 1, err
}

func (q *Querier) CreateSession(ctx context.Context, userID int64, hash, createdAt, expiresAt string) error {
	return q.generated.CreateSession(ctx, dbgen.CreateSessionParams{
		UserID: userID, SessionHash: hash, CreatedAt: createdAt,
		ExpiresAt: expiresAt, LastSeenAt: createdAt,
	})
}

func (q *Querier) GetActiveSessionUser(ctx context.Context, hash, now string) (User, error) {
	row, err := q.generated.GetActiveSessionUser(ctx, dbgen.GetActiveSessionUserParams{
		SessionHash: hash, ExpiresAt: now,
	})
	if err != nil {
		return User{}, err
	}
	return User{
		ID: row.ID, PublicID: row.PublicID, Username: row.Username,
		UsernameNormalized: row.UsernameNormalized, DisplayName: row.DisplayName,
		PasswordHash: row.PasswordHash, DisabledAt: nullString(row.DisabledAt),
	}, nil
}

func (q *Querier) TouchSession(ctx context.Context, hash, now string) error {
	_, err := q.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE session_hash = ?`, now, hash)
	return err
}

func (q *Querier) RevokeSession(ctx context.Context, hash, now string) error {
	return q.generated.RevokeSession(ctx, dbgen.RevokeSessionParams{RevokedAt: now, SessionHash: hash})
}

func nullString(value any) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	if text, ok := value.(string); ok {
		return sql.NullString{String: text, Valid: true}
	}
	if bytes, ok := value.([]byte); ok {
		return sql.NullString{String: string(bytes), Valid: true}
	}
	return sql.NullString{}
}

func (q *Querier) CreateAuditEvent(ctx context.Context, actorUserID, organizationID any, action string, targetType, targetID, reason, metadata any) error {
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO audit_log (actor_user_id, organization_id, action, target_type, target_id, reason, metadata_json, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		actorUserID, organizationID, action, targetType, targetID, reason, metadata, Now())
	return err
}

func (q *Querier) ListRecentAuditEvents(ctx context.Context, limit int64) ([]AuditEvent, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT id, actor_user_id, organization_id, action, target_type, target_id, reason, metadata_json, created_at
		FROM audit_log ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var events []AuditEvent
	for rows.Next() {
		var event AuditEvent
		if err := rows.Scan(&event.ID, &event.ActorUserID, &event.OrganizationID, &event.Action, &event.TargetType, &event.TargetID, &event.Reason, &event.MetadataJSON, &event.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func (q *Querier) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := q.db.QueryRowContext(ctx, `SELECT value FROM instance_settings WHERE key = ?`, key).Scan(&value)
	return value, err
}

func (q *Querier) CreateFirstOwner(ctx context.Context, input FirstOwnerInput) (User, Organization, error) {
	tx, err := q.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return User{}, Organization{}, err
	}
	defer tx.Rollback()

	var ownerCount int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM instance_roles WHERE role = 'instance_owner' AND revoked_at IS NULL`).Scan(&ownerCount); err != nil {
		return User{}, Organization{}, err
	}
	if ownerCount != 0 {
		return User{}, Organization{}, ErrOwnerExists
	}

	now := Now()
	result, err := tx.ExecContext(ctx, `
		INSERT INTO users (public_id, username, username_normalized, display_name, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		input.UserPublicID, input.Username, input.UsernameNormalized, input.DisplayName, input.PasswordHash, now, now)
	if err != nil {
		return User{}, Organization{}, fmt.Errorf("create owner: %w", err)
	}
	userID, err := result.LastInsertId()
	if err != nil {
		return User{}, Organization{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO instance_roles (user_id, role, created_at, created_by_user_id)
		VALUES (?, 'instance_owner', ?, ?)`, userID, now, userID); err != nil {
		return User{}, Organization{}, err
	}
	orgResult, err := tx.ExecContext(ctx, `
		INSERT INTO organizations (public_id, slug, name, created_by_user_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		input.OrganizationPublicID, input.OrganizationSlug, input.OrganizationName, userID, now, now)
	if err != nil {
		return User{}, Organization{}, fmt.Errorf("create default organization: %w", err)
	}
	organizationID, err := orgResult.LastInsertId()
	if err != nil {
		return User{}, Organization{}, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO organization_members (organization_id, user_id, role, created_at, created_by_user_id)
		VALUES (?, ?, 'owner', ?, ?)`, organizationID, userID, now, userID); err != nil {
		return User{}, Organization{}, err
	}
	settings := map[string]string{
		"registration_enabled": fmt.Sprintf("%t", input.RegistrationEnabled),
		"invite_only":          fmt.Sprintf("%t", input.InviteOnly),
		"instance_name":        input.InstanceName,
	}
	for key, value := range settings {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO instance_settings (key, value, updated_at, updated_by_user_id) VALUES (?, ?, ?, ?)`,
			key, value, now, userID); err != nil {
			return User{}, Organization{}, err
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO audit_log (actor_user_id, organization_id, action, target_type, target_id, metadata_json, created_at)
		VALUES (?, ?, ?, 'user', ?, ?, ?)`,
		userID, organizationID, input.AuditAction, input.UserPublicID, `{"source":"`+input.AuditSource+`"}`, now); err != nil {
		return User{}, Organization{}, err
	}
	if err := tx.Commit(); err != nil {
		return User{}, Organization{}, err
	}
	return User{
			ID: userID, PublicID: input.UserPublicID, Username: input.Username,
			UsernameNormalized: input.UsernameNormalized, DisplayName: input.DisplayName,
			PasswordHash: input.PasswordHash,
		}, Organization{
			ID: organizationID, PublicID: input.OrganizationPublicID, Slug: input.OrganizationSlug,
			Name: input.OrganizationName, Role: "owner",
		}, nil
}

type FirstOwnerInput struct {
	UserPublicID         string
	Username             string
	UsernameNormalized   string
	DisplayName          string
	PasswordHash         string
	OrganizationPublicID string
	OrganizationSlug     string
	OrganizationName     string
	InstanceName         string
	RegistrationEnabled  bool
	InviteOnly           bool
	AuditAction          string
	AuditSource          string
}

var ErrOwnerExists = errors.New("instance owner already exists")

func NormalizeUsername(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
