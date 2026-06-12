package db

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var ErrVisitLimitReached = errors.New("monthly visit safety limit reached")

type PublicCampaign struct {
	Campaign             Campaign
	Settings             CampaignSettings
	OrganizationDisabled bool
}

type RecordVisitInput struct {
	PublicID       string
	CampaignID     int64
	OrganizationID int64
	TokenHash      string
	ReferrerDomain string
	CoarseBrowser  string
	CoarseOS       string
	CountRaw       bool
	CountUnique    bool
	CollectToken   bool
	CreatedAt      time.Time
}

type CampaignVisitStats struct {
	RawTotal          int64
	UniqueTokenTotal  int64
	CurrentMonthTotal int64
	LastVisitAt       sql.NullString
}

func (q *Querier) GetPublicCampaignByID(ctx context.Context, publicID string) (PublicCampaign, error) {
	return q.scanPublicCampaign(ctx, `c.public_id=?`, publicID)
}

func (q *Querier) GetPublicCampaignBySlug(ctx context.Context, orgSlug, campaignSlug string) (PublicCampaign, error) {
	return q.scanPublicCampaign(ctx, `o.slug=? AND c.slug=?`, orgSlug, campaignSlug)
}

func (q *Querier) scanPublicCampaign(ctx context.Context, predicate string, args ...any) (PublicCampaign, error) {
	var result PublicCampaign
	query := `SELECT c.id,c.public_id,c.organization_id,o.public_id,o.name,o.slug,c.slug,c.name,c.description,c.status,c.public_link_enabled,c.created_by_user_id,u.username,c.created_at,c.updated_at,c.archived_at,c.disabled_at,NULL,
		(SELECT COUNT(*) FROM campaign_members owners WHERE owners.campaign_id=c.id AND owners.role='owner'),
		o.disabled_at,
		cs.collect_install_token,cs.hash_install_token,cs.count_raw_visits,cs.count_unique_token_visits,cs.collect_referrer_domain,cs.collect_coarse_browser,cs.collect_coarse_os,cs.public_language_default,cs.show_privacy_notice,cs.updated_at,cs.updated_by_user_id
		FROM campaigns c
		JOIN organizations o ON o.id=c.organization_id
		JOIN users u ON u.id=c.created_by_user_id
		JOIN campaign_settings cs ON cs.campaign_id=c.id
		WHERE ` + predicate
	var orgDisabled sql.NullString
	err := q.db.QueryRowContext(ctx, query, args...).Scan(
		&result.Campaign.ID, &result.Campaign.PublicID, &result.Campaign.OrganizationID,
		&result.Campaign.OrganizationPublicID, &result.Campaign.OrganizationName, &result.Campaign.OrganizationSlug,
		&result.Campaign.Slug, &result.Campaign.Name, &result.Campaign.Description, &result.Campaign.Status,
		&result.Campaign.PublicLinkEnabled, &result.Campaign.CreatedByUserID, &result.Campaign.CreatorUsername,
		&result.Campaign.CreatedAt, &result.Campaign.UpdatedAt, &result.Campaign.ArchivedAt,
		&result.Campaign.DisabledAt, &result.Campaign.ExplicitRole, &result.Campaign.ExplicitOwnerCount,
		&orgDisabled,
		&result.Settings.CollectInstallToken, &result.Settings.HashInstallToken, &result.Settings.CountRawVisits,
		&result.Settings.CountUniqueTokenVisits, &result.Settings.CollectReferrerDomain,
		&result.Settings.CollectCoarseBrowser, &result.Settings.CollectCoarseOS,
		&result.Settings.PublicLanguageDefault, &result.Settings.ShowPrivacyNotice,
		&result.Settings.UpdatedAt, &result.Settings.UpdatedByUserID,
	)
	result.OrganizationDisabled = orgDisabled.Valid
	return result, err
}

func (c PublicCampaign) Available() bool {
	return !c.OrganizationDisabled &&
		!c.Campaign.DisabledAt.Valid &&
		c.Campaign.Status == "active" &&
		c.Campaign.PublicLinkEnabled
}

func (q *Querier) RecordCampaignVisit(ctx context.Context, in RecordVisitInput) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if !in.CountRaw && !in.CountUnique && in.ReferrerDomain == "" && in.CoarseBrowser == "" && in.CoarseOS == "" {
		return tx.Commit()
	}
	createdAt := in.CreatedAt.UTC()
	monthStart := time.Date(createdAt.Year(), createdAt.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)
	var limit, current int64
	if err = tx.QueryRowContext(ctx, `SELECT max_monthly_visits FROM organization_limits WHERE organization_id=?`, in.OrganizationID).Scan(&limit); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_visits cv JOIN campaigns c ON c.id=cv.campaign_id WHERE c.organization_id=? AND cv.created_at>=? AND cv.created_at<?`,
		in.OrganizationID, monthStart.Format(time.RFC3339Nano), monthEnd.Format(time.RFC3339Nano)).Scan(&current); err != nil {
		return err
	}
	if current >= limit {
		return ErrVisitLimitReached
	}
	tokenHash := any(nil)
	unique := false
	if in.CollectToken && in.TokenHash != "" {
		tokenHash = in.TokenHash
		if in.CountUnique {
			var exists int
			if err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM campaign_visits WHERE campaign_id=? AND install_token_hash=? AND counted_as_unique_token_visit=1)`, in.CampaignID, in.TokenHash).Scan(&exists); err != nil {
				return err
			}
			unique = exists == 0
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO campaign_visits(public_id,campaign_id,install_token_hash,visit_kind,counted_as_unique_token_visit,counted_as_raw_visit,referrer_domain,coarse_browser,coarse_os,created_at)
		VALUES(?,?,?,'public_page',?,?,?,?,?,?)`,
		in.PublicID, in.CampaignID, tokenHash, unique, in.CountRaw, nullableText(in.ReferrerDomain),
		nullableText(in.CoarseBrowser), nullableText(in.CoarseOS), createdAt.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Querier) CampaignVisitStats(ctx context.Context, campaignID int64, now time.Time) (CampaignVisitStats, error) {
	monthStart := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	monthEnd := monthStart.AddDate(0, 1, 0)
	var stats CampaignVisitStats
	err := q.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(counted_as_raw_visit),0),
		COALESCE(SUM(counted_as_unique_token_visit),0),
		COALESCE(SUM(CASE WHEN created_at>=? AND created_at<? THEN counted_as_raw_visit ELSE 0 END),0),
		MAX(created_at)
		FROM campaign_visits WHERE campaign_id=?`,
		monthStart.Format(time.RFC3339Nano), monthEnd.Format(time.RFC3339Nano), campaignID).
		Scan(&stats.RawTotal, &stats.UniqueTokenTotal, &stats.CurrentMonthTotal, &stats.LastVisitAt)
	return stats, err
}
