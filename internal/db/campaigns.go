package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrCampaignArchived  = errors.New("campaign is archived")
	ErrInvalidTransition = errors.New("invalid campaign status transition")
)

type Campaign struct {
	ID                     int64
	PublicID               string
	OrganizationID         int64
	OrganizationPublicID   string
	OrganizationName       string
	OrganizationSlug       string
	Slug                   string
	Name                   string
	Description            sql.NullString
	Status                 string
	PublicLinkEnabled      bool
	CreatedByUserID        int64
	CreatorUsername        string
	CreatedAt              string
	UpdatedAt              string
	ArchivedAt             sql.NullString
	DisabledAt             sql.NullString
	ExplicitRole           sql.NullString
	ExplicitOwnerCount     int64
	PrivacyOptionalEnabled bool
}

type CampaignSettings struct {
	CollectInstallToken    bool
	HashInstallToken       bool
	CountRawVisits         bool
	CountUniqueTokenVisits bool
	CollectReferrerDomain  bool
	CollectCoarseBrowser   bool
	CollectCoarseOS        bool
	PublicLanguageDefault  string
	ShowPrivacyNotice      bool
	RetentionEnabled       bool
	RetentionDays          sql.NullInt64
	UpdatedAt              string
	UpdatedByUserID        sql.NullInt64
}

type CampaignMember struct {
	UserID      int64
	PublicID    string
	Username    string
	DisplayName string
	OrgRole     string
	Role        sql.NullString
}

type CreateCampaignInput struct {
	PublicID, Name, Slug, Description, Language, PrivacyPreset string
	OrganizationID, CreatedBy                                  int64
}

func (q *Querier) CreateCampaign(ctx context.Context, in CreateCampaignInput) (Campaign, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return Campaign{}, err
	}
	defer tx.Rollback()
	var limit, count int64
	if err = tx.QueryRowContext(ctx, `SELECT max_campaigns FROM organization_limits WHERE organization_id=?`, in.OrganizationID).Scan(&limit); err != nil {
		return Campaign{}, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaigns WHERE organization_id=?`, in.OrganizationID).Scan(&count); err != nil {
		return Campaign{}, err
	}
	if count >= limit {
		return Campaign{}, ErrLimitReached
	}
	now := Now()
	res, err := tx.ExecContext(ctx, `INSERT INTO campaigns(public_id,organization_id,slug,name,description,status,created_by_user_id,created_at,updated_at)
		VALUES(?,?,?,?,?,'draft',?,?,?)`, in.PublicID, in.OrganizationID, in.Slug, in.Name, nullableText(in.Description), in.CreatedBy, now, now)
	if err != nil {
		return Campaign{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Campaign{}, err
	}
	settings := PrivacyPreset(in.PrivacyPreset)
	if in.Language == "de" || in.Language == "es" {
		settings.PublicLanguageDefault = in.Language
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO campaign_settings(campaign_id,collect_install_token,hash_install_token,count_raw_visits,count_unique_token_visits,collect_referrer_domain,collect_coarse_browser,collect_coarse_os,public_language_default,show_privacy_notice,updated_at,updated_by_user_id)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, settings.CollectInstallToken, true, settings.CountRawVisits, settings.CountUniqueTokenVisits, settings.CollectReferrerDomain, settings.CollectCoarseBrowser, settings.CollectCoarseOS, settings.PublicLanguageDefault, settings.ShowPrivacyNotice, now, in.CreatedBy); err != nil {
		return Campaign{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO campaign_members(campaign_id,user_id,role,created_at,created_by_user_id) VALUES(?,?,'owner',?,?)`, id, in.CreatedBy, now, in.CreatedBy); err != nil {
		return Campaign{}, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id,created_at) VALUES(?,?,'campaign_created','campaign',?,?)`, in.CreatedBy, in.OrganizationID, in.PublicID, now); err != nil {
		return Campaign{}, err
	}
	if err = tx.Commit(); err != nil {
		return Campaign{}, err
	}
	return Campaign{ID: id, PublicID: in.PublicID, OrganizationID: in.OrganizationID, Slug: in.Slug, Name: in.Name, Description: nullString(nullableText(in.Description)), Status: "draft", CreatedByUserID: in.CreatedBy, CreatedAt: now, UpdatedAt: now, ExplicitRole: sql.NullString{String: "owner", Valid: true}, ExplicitOwnerCount: 1}, nil
}

func PrivacyPreset(preset string) CampaignSettings {
	settings := CampaignSettings{
		CollectInstallToken: true, HashInstallToken: true, CountRawVisits: true,
		CountUniqueTokenVisits: true, PublicLanguageDefault: "en", ShowPrivacyNotice: true,
	}
	if preset == "balanced" {
		settings.CollectReferrerDomain = true
		settings.CollectCoarseBrowser = true
		settings.CollectCoarseOS = true
	}
	return settings
}

func (q *Querier) CountCampaigns(ctx context.Context, orgID int64) (int64, error) {
	var count int64
	err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaigns WHERE organization_id=?`, orgID).Scan(&count)
	return count, err
}

func (q *Querier) ListCampaignsForUser(ctx context.Context, orgID, userID int64) ([]Campaign, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT c.id,c.public_id,c.organization_id,o.public_id,o.name,o.slug,c.slug,c.name,c.description,c.status,c.public_link_enabled,c.created_by_user_id,u.username,c.created_at,c.updated_at,c.archived_at,c.disabled_at,cm.role,
		(SELECT COUNT(*) FROM campaign_members owners WHERE owners.campaign_id=c.id AND owners.role='owner')
		FROM campaigns c
		JOIN organizations o ON o.id=c.organization_id
		JOIN users u ON u.id=c.created_by_user_id
		LEFT JOIN campaign_members cm ON cm.campaign_id=c.id AND cm.user_id=?
		WHERE c.organization_id=? AND (
			EXISTS(SELECT 1 FROM organization_members om WHERE om.organization_id=c.organization_id AND om.user_id=? AND om.role IN ('owner','admin'))
			OR cm.user_id IS NOT NULL
		) ORDER BY c.id DESC`, userID, orgID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (q *Querier) GetCampaignByPublicID(ctx context.Context, orgPublicID, campaignPublicID string) (Campaign, error) {
	return scanCampaign(q.db.QueryRowContext(ctx, `SELECT c.id,c.public_id,c.organization_id,o.public_id,o.name,o.slug,c.slug,c.name,c.description,c.status,c.public_link_enabled,c.created_by_user_id,u.username,c.created_at,c.updated_at,c.archived_at,c.disabled_at,NULL,
		(SELECT COUNT(*) FROM campaign_members owners WHERE owners.campaign_id=c.id AND owners.role='owner')
		FROM campaigns c JOIN organizations o ON o.id=c.organization_id JOIN users u ON u.id=c.created_by_user_id
		WHERE o.public_id=? AND c.public_id=?`, orgPublicID, campaignPublicID))
}

type campaignScanner interface{ Scan(...any) error }

func scanCampaign(row campaignScanner) (Campaign, error) {
	var c Campaign
	err := row.Scan(&c.ID, &c.PublicID, &c.OrganizationID, &c.OrganizationPublicID, &c.OrganizationName, &c.OrganizationSlug, &c.Slug, &c.Name, &c.Description, &c.Status, &c.PublicLinkEnabled, &c.CreatedByUserID, &c.CreatorUsername, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.DisabledAt, &c.ExplicitRole, &c.ExplicitOwnerCount)
	return c, err
}

func (q *Querier) CampaignRole(ctx context.Context, campaignID, userID int64) (string, error) {
	var orgRole string
	err := q.db.QueryRowContext(ctx, `SELECT om.role FROM campaigns c JOIN organization_members om ON om.organization_id=c.organization_id WHERE c.id=? AND om.user_id=?`, campaignID, userID).Scan(&orgRole)
	if err == nil && (orgRole == "owner" || orgRole == "admin") {
		return "owner", nil
	}
	var role string
	err = q.db.QueryRowContext(ctx, `SELECT role FROM campaign_members WHERE campaign_id=? AND user_id=?`, campaignID, userID).Scan(&role)
	return role, err
}

func (q *Querier) GetCampaignSettings(ctx context.Context, campaignID int64) (CampaignSettings, error) {
	var s CampaignSettings
	err := q.db.QueryRowContext(ctx, `SELECT collect_install_token,hash_install_token,count_raw_visits,count_unique_token_visits,collect_referrer_domain,collect_coarse_browser,collect_coarse_os,public_language_default,show_privacy_notice,retention_enabled,retention_days,updated_at,updated_by_user_id FROM campaign_settings WHERE campaign_id=?`, campaignID).
		Scan(&s.CollectInstallToken, &s.HashInstallToken, &s.CountRawVisits, &s.CountUniqueTokenVisits, &s.CollectReferrerDomain, &s.CollectCoarseBrowser, &s.CollectCoarseOS, &s.PublicLanguageDefault, &s.ShowPrivacyNotice, &s.RetentionEnabled, &s.RetentionDays, &s.UpdatedAt, &s.UpdatedByUserID)
	return s, err
}

func (q *Querier) UpdateCampaign(ctx context.Context, campaign Campaign, actorID int64) error {
	if campaign.Status == "archived" {
		return ErrCampaignArchived
	}
	res, err := q.db.ExecContext(ctx, `UPDATE campaigns SET name=?,slug=?,description=?,public_link_enabled=?,updated_at=? WHERE id=? AND status!='archived'`,
		campaign.Name, campaign.Slug, nullableText(campaign.Description.String), campaign.PublicLinkEnabled, Now(), campaign.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrCampaignArchived
	}
	return q.CreateAuditEvent(ctx, actorID, campaign.OrganizationID, "campaign_updated", "campaign", campaign.PublicID, nil, nil)
}

func (q *Querier) UpdateCampaignPrivacy(ctx context.Context, campaign Campaign, s CampaignSettings, actorID int64) error {
	if campaign.Status == "archived" {
		return ErrCampaignArchived
	}
	if s.PublicLanguageDefault != "en" && s.PublicLanguageDefault != "de" && s.PublicLanguageDefault != "es" {
		return ErrForbidden
	}
	_, err := q.db.ExecContext(ctx, `UPDATE campaign_settings SET collect_install_token=?,hash_install_token=1,count_raw_visits=?,count_unique_token_visits=?,collect_referrer_domain=?,collect_coarse_browser=?,collect_coarse_os=?,public_language_default=?,show_privacy_notice=?,retention_enabled=?,retention_days=?,updated_at=?,updated_by_user_id=? WHERE campaign_id=?`,
		s.CollectInstallToken, s.CountRawVisits, s.CountUniqueTokenVisits, s.CollectReferrerDomain, s.CollectCoarseBrowser, s.CollectCoarseOS, s.PublicLanguageDefault, s.ShowPrivacyNotice, s.RetentionEnabled, nullableInt64(s.RetentionDays), Now(), actorID, campaign.ID)
	if err != nil {
		return err
	}
	return q.CreateAuditEvent(ctx, actorID, campaign.OrganizationID, "campaign_privacy_updated", "campaign", campaign.PublicID, nil, nil)
}

func (q *Querier) ChangeCampaignStatus(ctx context.Context, campaign Campaign, next string, actorID int64) error {
	allowed := (campaign.Status == "draft" && next == "active") ||
		(campaign.Status == "active" && next == "paused") ||
		(campaign.Status == "paused" && next == "active") ||
		(campaign.Status != "archived" && next == "archived")
	if !allowed {
		return ErrInvalidTransition
	}
	var archived any
	if next == "archived" {
		archived = Now()
	}
	if _, err := q.db.ExecContext(ctx, `UPDATE campaigns SET status=?,archived_at=?,updated_at=? WHERE id=?`, next, archived, Now(), campaign.ID); err != nil {
		return err
	}
	action := "campaign_status_updated"
	if next == "archived" {
		action = "campaign_archived"
	}
	return q.CreateAuditEvent(ctx, actorID, campaign.OrganizationID, action, "campaign", campaign.PublicID, nil, fmt.Sprintf(`{"status":%q}`, next))
}

func (q *Querier) ListCampaignAccess(ctx context.Context, campaignID, orgID int64) ([]CampaignMember, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT u.id,u.public_id,u.username,u.display_name,om.role,cm.role
		FROM organization_members om JOIN users u ON u.id=om.user_id
		LEFT JOIN campaign_members cm ON cm.user_id=u.id AND cm.campaign_id=?
		WHERE om.organization_id=? ORDER BY u.username`, campaignID, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CampaignMember
	for rows.Next() {
		var m CampaignMember
		if err := rows.Scan(&m.UserID, &m.PublicID, &m.Username, &m.DisplayName, &m.OrgRole, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (q *Querier) SetCampaignMember(ctx context.Context, campaign Campaign, userPublicID, role string, actorID int64) error {
	if campaign.Status == "archived" {
		return ErrCampaignArchived
	}
	if role != "owner" && role != "editor" && role != "analyst" && role != "viewer" {
		return ErrForbidden
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID int64
	if err = tx.QueryRowContext(ctx, `SELECT u.id FROM users u JOIN organization_members om ON om.user_id=u.id WHERE u.public_id=? AND om.organization_id=?`, userPublicID, campaign.OrganizationID).Scan(&userID); err != nil {
		return ErrForbidden
	}
	var current sql.NullString
	_ = tx.QueryRowContext(ctx, `SELECT role FROM campaign_members WHERE campaign_id=? AND user_id=?`, campaign.ID, userID).Scan(&current)
	if current.Valid && current.String == "owner" && role != "owner" {
		var owners int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_members WHERE campaign_id=? AND role='owner'`, campaign.ID).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO campaign_members(campaign_id,user_id,role,created_at,created_by_user_id) VALUES(?,?,?,?,?)
		ON CONFLICT(campaign_id,user_id) DO UPDATE SET role=excluded.role,created_by_user_id=excluded.created_by_user_id`, campaign.ID, userID, role, Now(), actorID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id,metadata_json,created_at) VALUES(?,?,'campaign_access_updated','campaign',?,?,?)`, actorID, campaign.OrganizationID, campaign.PublicID, fmt.Sprintf(`{"user":%q,"role":%q}`, userPublicID, role), Now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Querier) RemoveCampaignMember(ctx context.Context, campaign Campaign, userPublicID string, actorID int64) error {
	if campaign.Status == "archived" {
		return ErrCampaignArchived
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var userID int64
	var role string
	if err = tx.QueryRowContext(ctx, `SELECT cm.user_id,cm.role FROM campaign_members cm JOIN users u ON u.id=cm.user_id WHERE cm.campaign_id=? AND u.public_id=?`, campaign.ID, userPublicID).Scan(&userID, &role); err != nil {
		return err
	}
	if role == "owner" {
		var owners int
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_members WHERE campaign_id=? AND role='owner'`, campaign.ID).Scan(&owners); err != nil {
			return err
		}
		if owners <= 1 {
			return ErrLastOwner
		}
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM campaign_members WHERE campaign_id=? AND user_id=?`, campaign.ID, userID); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id,metadata_json,created_at) VALUES(?,?,'campaign_access_removed','campaign',?,?,?)`, actorID, campaign.OrganizationID, campaign.PublicID, fmt.Sprintf(`{"user":%q}`, userPublicID), Now()); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Querier) ListInstanceCampaigns(ctx context.Context) ([]Campaign, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT c.id,c.public_id,c.organization_id,o.public_id,o.name,o.slug,c.slug,c.name,c.description,c.status,c.public_link_enabled,c.created_by_user_id,u.username,c.created_at,c.updated_at,c.archived_at,c.disabled_at,NULL,
		(SELECT COUNT(*) FROM campaign_members owners WHERE owners.campaign_id=c.id AND owners.role='owner'),
		(cs.collect_referrer_domain=1 OR cs.collect_coarse_browser=1 OR cs.collect_coarse_os=1)
		FROM campaigns c JOIN organizations o ON o.id=c.organization_id JOIN users u ON u.id=c.created_by_user_id
		JOIN campaign_settings cs ON cs.campaign_id=c.id ORDER BY c.id DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Campaign
	for rows.Next() {
		var c Campaign
		if err := rows.Scan(&c.ID, &c.PublicID, &c.OrganizationID, &c.OrganizationPublicID, &c.OrganizationName, &c.OrganizationSlug, &c.Slug, &c.Name, &c.Description, &c.Status, &c.PublicLinkEnabled, &c.CreatedByUserID, &c.CreatorUsername, &c.CreatedAt, &c.UpdatedAt, &c.ArchivedAt, &c.DisabledAt, &c.ExplicitRole, &c.ExplicitOwnerCount, &c.PrivacyOptionalEnabled); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (q *Querier) SetCampaignDisabled(ctx context.Context, publicID string, disabled bool, actorID int64) error {
	value := any(nil)
	action := "campaign_enabled"
	if disabled {
		value = Now()
		action = "campaign_disabled"
	}
	var orgID int64
	err := q.db.QueryRowContext(ctx, `UPDATE campaigns SET disabled_at=?,updated_at=? WHERE public_id=? RETURNING organization_id`, value, Now(), publicID).Scan(&orgID)
	if err != nil {
		return err
	}
	return q.CreateAuditEvent(ctx, actorID, orgID, action, "campaign", publicID, nil, nil)
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
