package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
)

var (
	ErrLimitReached      = errors.New("safety limit reached")
	ErrForbidden         = errors.New("forbidden")
	ErrLastOwner         = errors.New("organization must retain an owner")
	ErrInviteUnavailable = errors.New("invite unavailable")
	ErrAlreadyMember     = errors.New("already a member")
)

func HashInviteCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func (q *Querier) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT key, value FROM instance_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (q *Querier) GetOrganizationByPublicID(ctx context.Context, publicID string) (Organization, error) {
	var o Organization
	err := q.db.QueryRowContext(ctx, `SELECT o.id,o.public_id,o.slug,o.name,o.created_at,o.disabled_at,
		(SELECT COUNT(*) FROM organization_members WHERE organization_id=o.id)
		FROM organizations o WHERE o.public_id=?`, publicID).
		Scan(&o.ID, &o.PublicID, &o.Slug, &o.Name, &o.CreatedAt, &o.DisabledAt, &o.MemberCount)
	return o, err
}

func (q *Querier) OrganizationRole(ctx context.Context, userID, orgID int64) (string, error) {
	var role string
	err := q.db.QueryRowContext(ctx, `SELECT role FROM organization_members WHERE user_id=? AND organization_id=?`, userID, orgID).Scan(&role)
	return role, err
}

func (q *Querier) GetOrganizationLimits(ctx context.Context, orgID int64) (OrganizationLimits, error) {
	var l OrganizationLimits
	err := q.db.QueryRowContext(ctx, `SELECT max_campaigns,max_members,max_active_invites,max_monthly_visits,max_monthly_submissions
		FROM organization_limits WHERE organization_id=?`, orgID).Scan(&l.MaxCampaigns, &l.MaxMembers, &l.MaxActiveInvites, &l.MaxMonthlyVisits, &l.MaxMonthlySubmissions)
	return l, err
}

func (q *Querier) ListOrganizationMembers(ctx context.Context, orgID int64) ([]OrganizationMember, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT u.id,u.public_id,u.username,u.display_name,om.role,om.created_at
		FROM organization_members om JOIN users u ON u.id=om.user_id WHERE om.organization_id=? ORDER BY om.role,u.username`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []OrganizationMember
	for rows.Next() {
		var m OrganizationMember
		if err := rows.Scan(&m.UserID, &m.PublicID, &m.Username, &m.DisplayName, &m.Role, &m.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (q *Querier) CountOrganizationsCreatedByUser(ctx context.Context, userID int64) (int, error) {
	var n int
	err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations WHERE created_by_user_id=? AND disabled_at IS NULL`, userID).Scan(&n)
	return n, err
}

type CreateOrganizationInput struct {
	PublicID, Slug, Name string
	UserID               int64
	Limits               DefaultLimits
}

func (q *Querier) CreateOrganization(ctx context.Context, in CreateOrganizationInput) (Organization, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return Organization{}, err
	}
	defer tx.Rollback()
	var raw string
	if err := tx.QueryRowContext(ctx, `SELECT value FROM instance_settings WHERE key='default_max_organizations_per_user'`).Scan(&raw); err != nil {
		return Organization{}, err
	}
	limit, _ := strconv.Atoi(raw)
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM organizations WHERE created_by_user_id=? AND disabled_at IS NULL`, in.UserID).Scan(&count); err != nil {
		return Organization{}, err
	}
	if count >= limit {
		return Organization{}, ErrLimitReached
	}
	now := Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO organizations(public_id,slug,name,created_by_user_id,created_at,updated_at)VALUES(?,?,?,?,?,?)`, in.PublicID, in.Slug, in.Name, in.UserID, now, now)
	if err != nil {
		return Organization{}, err
	}
	id, _ := res.LastInsertId()
	if _, err = tx.ExecContext(ctx, `INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id)VALUES(?,?,'owner',?,?)`, id, in.UserID, now, in.UserID); err != nil {
		return Organization{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO organization_limits VALUES(?,?,?,?,?,?,?,?)`, id, in.Limits.MaxCampaignsPerOrg, in.Limits.MaxMembersPerOrg, in.Limits.MaxActiveInvitesPerOrg, in.Limits.MaxMonthlyVisitsPerOrg, in.Limits.MaxMonthlySubmissionsPerOrg, now, in.UserID); err != nil {
		return Organization{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id,created_at)VALUES(?,?,'organization_created','organization',?,?)`, in.UserID, id, in.PublicID, now); err != nil {
		return Organization{}, err
	}
	if err = tx.Commit(); err != nil {
		return Organization{}, err
	}
	return Organization{ID: id, PublicID: in.PublicID, Slug: in.Slug, Name: in.Name, Role: "owner", CreatedAt: now, MemberCount: 1}, nil
}

type CreateInviteInput struct {
	PublicID, CodeHash, Role, ExpiresAt string
	OrganizationID, CreatedBy           int64
	MaxUses                             int
}

func (q *Querier) CreateInvite(ctx context.Context, in CreateInviteInput) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var max, active int
	if err = tx.QueryRowContext(ctx, `SELECT max_active_invites FROM organization_limits WHERE organization_id=?`, in.OrganizationID).Scan(&max); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM invites WHERE organization_id=? AND revoked_at IS NULL AND expires_at>? AND used_count<max_uses`, in.OrganizationID, Now()).Scan(&active); err != nil {
		return err
	}
	if active >= max {
		return ErrLimitReached
	}
	now := Now()
	if _, err = tx.ExecContext(ctx, `INSERT INTO invites(public_id,code_hash,organization_id,role,max_uses,expires_at,created_by_user_id,created_at)VALUES(?,?,?,?,?,?,?,?)`, in.PublicID, in.CodeHash, in.OrganizationID, in.Role, in.MaxUses, in.ExpiresAt, in.CreatedBy, now); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id,created_at)VALUES(?,?,'invite_created','invite',?,?)`, in.CreatedBy, in.OrganizationID, in.PublicID, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Querier) GetInviteByCode(ctx context.Context, code string) (Invite, error) {
	var i Invite
	err := q.db.QueryRowContext(ctx, `SELECT i.id,i.public_id,i.code_hash,i.organization_id,o.public_id,o.name,i.role,i.max_uses,i.used_count,i.expires_at,i.created_at,i.revoked_at
		FROM invites i JOIN organizations o ON o.id=i.organization_id WHERE i.code_hash=?`, HashInviteCode(code)).
		Scan(&i.ID, &i.PublicID, &i.CodeHash, &i.OrganizationID, &i.OrganizationPublicID, &i.OrganizationName, &i.Role, &i.MaxUses, &i.UsedCount, &i.ExpiresAt, &i.CreatedAt, &i.RevokedAt)
	return i, err
}

func (q *Querier) ListInvites(ctx context.Context, orgID int64) ([]Invite, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT id,public_id,code_hash,organization_id,role,max_uses,used_count,expires_at,created_at,revoked_at FROM invites WHERE organization_id=? ORDER BY id DESC`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Invite
	for rows.Next() {
		var i Invite
		if err := rows.Scan(&i.ID, &i.PublicID, &i.CodeHash, &i.OrganizationID, &i.Role, &i.MaxUses, &i.UsedCount, &i.ExpiresAt, &i.CreatedAt, &i.RevokedAt); err != nil {
			return nil, err
		}
		out = append(out, i)
	}
	return out, rows.Err()
}

func (q *Querier) RevokeInvite(ctx context.Context, invitePublicID string, orgID, actorID int64) error {
	res, err := q.db.ExecContext(ctx, `UPDATE invites SET revoked_at=? WHERE public_id=? AND organization_id=? AND revoked_at IS NULL`, Now(), invitePublicID, orgID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return q.CreateAuditEvent(ctx, actorID, orgID, "invite_revoked", "invite", invitePublicID, nil, nil)
}

func (q *Querier) AcceptInvite(ctx context.Context, code string, userID int64) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := Now()
	var id, orgID, maxUses, used, maxMembers int64
	var publicID, role, expires string
	var revoked sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT i.id,i.public_id,i.organization_id,i.role,i.max_uses,i.used_count,i.expires_at,i.revoked_at,l.max_members
		FROM invites i JOIN organization_limits l ON l.organization_id=i.organization_id JOIN organizations o ON o.id=i.organization_id
		WHERE i.code_hash=? AND o.disabled_at IS NULL`, HashInviteCode(code)).Scan(&id, &publicID, &orgID, &role, &maxUses, &used, &expires, &revoked, &maxMembers)
	if err != nil {
		return ErrInviteUnavailable
	}
	if revoked.Valid || expires <= now || used >= maxUses {
		return ErrInviteUnavailable
	}
	var exists int
	if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM organization_members WHERE organization_id=? AND user_id=?)`, orgID, userID).Scan(&exists); err != nil {
		return err
	}
	if exists == 1 {
		return ErrAlreadyMember
	}
	var members int64
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM organization_members WHERE organization_id=?`, orgID).Scan(&members); err != nil {
		return err
	}
	if members >= maxMembers {
		return ErrLimitReached
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id)VALUES(?,?,?,?,NULL)`, orgID, userID, role, now); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `UPDATE invites SET used_count=used_count+1 WHERE id=? AND used_count < max_uses AND revoked_at IS NULL AND expires_at > ?`, id, now)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrInviteUnavailable
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id,created_at)VALUES(?,?,'invite_accepted','invite',?,?)`, userID, orgID, publicID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Querier) RemoveMember(ctx context.Context, orgID, targetID, actorID int64, actorRole string, isInstanceOwner bool) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var targetRole string
	if err = tx.QueryRowContext(ctx, `SELECT role FROM organization_members WHERE organization_id=? AND user_id=?`, orgID, targetID).Scan(&targetRole); err != nil {
		return err
	}
	if targetRole == "owner" && actorRole != "owner" && !isInstanceOwner {
		return ErrForbidden
	}
	if targetRole == "owner" {
		var owners int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM organization_members WHERE organization_id=? AND role='owner'`, orgID).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	if actorRole != "owner" && actorRole != "admin" && !isInstanceOwner {
		return ErrForbidden
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM organization_members WHERE organization_id=? AND user_id=?`, orgID, targetID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id,created_at)VALUES(?,?,'member_removed','user',(SELECT public_id FROM users WHERE id=?),?)`, actorID, orgID, targetID, Now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Querier) UpdateMemberRole(ctx context.Context, orgID, targetID, actorID int64, actorRole, newRole string, isInstanceOwner bool) error {
	if newRole != "owner" && newRole != "admin" && newRole != "member" && newRole != "viewer" {
		return ErrForbidden
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var current string
	if err = tx.QueryRowContext(ctx, `SELECT role FROM organization_members WHERE organization_id=? AND user_id=?`, orgID, targetID).Scan(&current); err != nil {
		return err
	}
	if (current == "owner" || newRole == "owner") && actorRole != "owner" && !isInstanceOwner {
		return ErrForbidden
	}
	if current == "owner" && newRole != "owner" {
		var owners int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM organization_members WHERE organization_id=? AND role='owner'`, orgID).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	if actorRole != "owner" && actorRole != "admin" && !isInstanceOwner {
		return ErrForbidden
	}
	if _, err = tx.ExecContext(ctx, `UPDATE organization_members SET role=? WHERE organization_id=? AND user_id=?`, newRole, orgID, targetID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id,metadata_json,created_at) VALUES(?,?,'member_role_updated','user',(SELECT public_id FROM users WHERE id=?),?,?)`, actorID, orgID, targetID, fmt.Sprintf(`{"role":%q}`, newRole), Now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Querier) UpdateOrganization(ctx context.Context, orgID, actorID int64, name, slug string) error {
	res, err := q.db.ExecContext(ctx, `UPDATE organizations SET name=?,slug=?,updated_at=? WHERE id=? AND disabled_at IS NULL`, name, slug, Now(), orgID)
	if err != nil {
		return err
	}
	count, _ := res.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return q.CreateAuditEvent(ctx, actorID, orgID, "organization_updated", "organization", fmt.Sprint(orgID), nil, nil)
}

func (q *Querier) CreateUser(ctx context.Context, publicID, username, normalized, email, emailNormalized, displayName, passwordHash string) (User, error) {
	now := Now()
	var emailV, emailNV any
	if email != "" {
		emailV = email
		emailNV = emailNormalized
	}
	res, err := q.db.ExecContext(ctx, `INSERT INTO users(public_id,username,username_normalized,email,email_normalized,display_name,password_hash,created_at,updated_at)VALUES(?,?,?,?,?,?,?,?,?)`, publicID, username, normalized, emailV, emailNV, displayName, passwordHash, now, now)
	if err != nil {
		return User{}, err
	}
	id, _ := res.LastInsertId()
	return User{ID: id, PublicID: publicID, Username: username, UsernameNormalized: normalized, DisplayName: displayName, PasswordHash: passwordHash, CreatedAt: now}, nil
}

func (q *Querier) ListInstanceUsers(ctx context.Context) ([]InstanceUser, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT u.id,u.public_id,u.username,u.display_name,u.email,u.created_at,u.disabled_at,(SELECT role FROM instance_roles WHERE user_id=u.id AND revoked_at IS NULL ORDER BY id LIMIT 1) FROM users u ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InstanceUser
	for rows.Next() {
		var u InstanceUser
		if err := rows.Scan(&u.ID, &u.PublicID, &u.Username, &u.DisplayName, &u.Email, &u.CreatedAt, &u.DisabledAt, &u.InstanceRole); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (q *Querier) ListInstanceOrganizations(ctx context.Context) ([]Organization, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT o.id,o.public_id,o.slug,o.name,o.created_at,o.disabled_at,(SELECT COUNT(*) FROM organization_members WHERE organization_id=o.id),(SELECT COUNT(*) FROM organization_members WHERE organization_id=o.id AND role='owner') FROM organizations o ORDER BY o.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Organization
	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.PublicID, &o.Slug, &o.Name, &o.CreatedAt, &o.DisabledAt, &o.MemberCount, &o.OwnerCount); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (q *Querier) SetUserDisabled(ctx context.Context, publicID string, disabled bool, actorID int64) error {
	if disabled {
		var last int
		err := q.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM users u JOIN instance_roles r ON r.user_id=u.id WHERE u.public_id=? AND r.role='instance_owner' AND r.revoked_at IS NULL AND u.disabled_at IS NULL) AND (SELECT COUNT(*) FROM users u JOIN instance_roles r ON r.user_id=u.id WHERE r.role='instance_owner' AND r.revoked_at IS NULL AND u.disabled_at IS NULL)=1`, publicID).Scan(&last)
		if err != nil {
			return err
		}
		if last == 1 {
			return ErrLastOwner
		}
	}
	value := any(nil)
	action := "user_enabled"
	if disabled {
		value = Now()
		action = "user_disabled"
	}
	res, err := q.db.ExecContext(ctx, `UPDATE users SET disabled_at=?,updated_at=? WHERE public_id=?`, value, Now(), publicID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return q.CreateAuditEvent(ctx, actorID, nil, action, "user", publicID, nil, nil)
}
func (q *Querier) SetOrganizationDisabled(ctx context.Context, publicID string, disabled bool, actorID int64) error {
	value := any(nil)
	action := "organization_enabled"
	if disabled {
		value = Now()
		action = "organization_disabled"
	}
	var id int64
	err := q.db.QueryRowContext(ctx, `UPDATE organizations SET disabled_at=?,updated_at=? WHERE public_id=? RETURNING id`, value, Now(), publicID).Scan(&id)
	if err != nil {
		return err
	}
	return q.CreateAuditEvent(ctx, actorID, id, action, "organization", publicID, nil, nil)
}
func (q *Querier) UpdateOrganizationLimits(ctx context.Context, orgID int64, l OrganizationLimits, actorID int64) error {
	_, err := q.db.ExecContext(ctx, `UPDATE organization_limits SET max_campaigns=?,max_members=?,max_active_invites=?,max_monthly_visits=?,max_monthly_submissions=?,updated_at=?,updated_by_user_id=? WHERE organization_id=?`, l.MaxCampaigns, l.MaxMembers, l.MaxActiveInvites, l.MaxMonthlyVisits, l.MaxMonthlySubmissions, Now(), actorID, orgID)
	if err != nil {
		return err
	}
	return q.CreateAuditEvent(ctx, actorID, orgID, "organization_limits_updated", "organization_limits", fmt.Sprint(orgID), nil, nil)
}
func (q *Querier) UpdateSettings(ctx context.Context, values map[string]string, actorID int64) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for k, v := range values {
		if _, err = tx.ExecContext(ctx, `INSERT INTO instance_settings(key,value,updated_at,updated_by_user_id)VALUES(?,?,?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at,updated_by_user_id=excluded.updated_by_user_id`, k, v, Now(), actorID); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,action,target_type,target_id,created_at)VALUES(?,'instance_settings_updated','instance_settings','global',?)`, actorID, Now()); err != nil {
		return err
	}
	return tx.Commit()
}
