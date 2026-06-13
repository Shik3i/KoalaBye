package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"time"
)

type AnalyticsOverview struct {
	RawVisits, UniqueVisits, Submissions        int64
	CurrentMonthVisits, CurrentMonthSubmissions int64
	LatestVisitAt, LatestSubmissionAt           sql.NullString
}

type DailyTrend struct {
	Day                                  string
	RawVisits, UniqueVisits, Submissions int64
}

type ValueCount struct {
	Value, Label string
	Count        int64
	Percentage   float64
}

type FieldSummary struct {
	PublicID, FieldType, Label string
	Archived                   bool
	Answered, Skipped          int64
	TotalSelections            int64
	Average                    float64
	Values                     []ValueCount
}

type MetadataCount struct {
	Value string
	Count int64
}

type CampaignAnalytics struct {
	Overview                        AnalyticsOverview
	Trend                           []DailyTrend
	Fields                          []FieldSummary
	Referrers, Browsers, OSFamilies []MetadataCount
}

func (q *Querier) CampaignAnalytics(ctx context.Context, campaignID int64, start *time.Time, now time.Time) (CampaignAnalytics, error) {
	var analytics CampaignAnalytics
	monthStart := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)
	err := q.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(counted_as_raw_visit),0),COALESCE(SUM(counted_as_unique_token_visit),0),
		COALESCE(SUM(CASE WHEN created_at>=? THEN counted_as_raw_visit ELSE 0 END),0),MAX(created_at)
		FROM campaign_visits WHERE campaign_id=?`, monthStart, campaignID).
		Scan(&analytics.Overview.RawVisits, &analytics.Overview.UniqueVisits, &analytics.Overview.CurrentMonthVisits, &analytics.Overview.LatestVisitAt)
	if err != nil {
		return analytics, err
	}
	err = q.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN submitted_at>=? THEN 1 ELSE 0 END),0),MAX(submitted_at)
		FROM campaign_submissions WHERE campaign_id=?`, monthStart, campaignID).
		Scan(&analytics.Overview.Submissions, &analytics.Overview.CurrentMonthSubmissions, &analytics.Overview.LatestSubmissionAt)
	if err != nil {
		return analytics, err
	}
	analytics.Trend, err = q.dailyTrend(ctx, campaignID, start, now)
	if err != nil {
		return analytics, err
	}
	analytics.Fields, err = q.fieldSummaries(ctx, campaignID, start)
	if err != nil {
		return analytics, err
	}
	analytics.Referrers, err = q.metadataSummary(ctx, campaignID, start, "referrer_domain")
	if err != nil {
		return analytics, err
	}
	analytics.Browsers, err = q.metadataSummary(ctx, campaignID, start, "coarse_browser")
	if err != nil {
		return analytics, err
	}
	analytics.OSFamilies, err = q.metadataSummary(ctx, campaignID, start, "coarse_os")
	return analytics, err
}

func (q *Querier) dailyTrend(ctx context.Context, campaignID int64, start *time.Time, now time.Time) ([]DailyTrend, error) {
	since := ""
	var startDay time.Time
	if start != nil {
		startDay = time.Date(start.UTC().Year(), start.UTC().Month(), start.UTC().Day(), 0, 0, 0, 0, time.UTC)
		since = startDay.Format(time.RFC3339Nano)
	}
	rows, err := q.db.QueryContext(ctx, `SELECT day,SUM(raw),SUM(unique_count),SUM(submissions) FROM (
		SELECT substr(created_at,1,10) day,counted_as_raw_visit raw,counted_as_unique_token_visit unique_count,0 submissions
		FROM campaign_visits WHERE campaign_id=? AND (?='' OR created_at>=?)
		UNION ALL
		SELECT substr(submitted_at,1,10) day,0,0,1 FROM campaign_submissions WHERE campaign_id=? AND (?='' OR submitted_at>=?)
	) GROUP BY day ORDER BY day`, campaignID, since, since, campaignID, since, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var trend []DailyTrend
	for rows.Next() {
		var point DailyTrend
		if err := rows.Scan(&point.Day, &point.RawVisits, &point.UniqueVisits, &point.Submissions); err != nil {
			return nil, err
		}
		trend = append(trend, point)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if start == nil {
		return trend, nil
	}
	byDay := make(map[string]DailyTrend, len(trend))
	for _, point := range trend {
		byDay[point.Day] = point
	}
	trend = nil
	end := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	for day := startDay; !day.After(end); day = day.AddDate(0, 0, 1) {
		key := day.Format("2006-01-02")
		point := byDay[key]
		point.Day = key
		trend = append(trend, point)
	}
	return trend, nil
}

func (q *Querier) metadataSummary(ctx context.Context, campaignID int64, start *time.Time, column string) ([]MetadataCount, error) {
	allowed := map[string]bool{"referrer_domain": true, "coarse_browser": true, "coarse_os": true}
	if !allowed[column] {
		return nil, ErrForbidden
	}
	since := ""
	if start != nil {
		since = start.UTC().Format(time.RFC3339Nano)
	}
	rows, err := q.db.QueryContext(ctx, `SELECT `+column+`,COUNT(*) FROM campaign_visits
		WHERE campaign_id=? AND `+column+` IS NOT NULL AND (?='' OR created_at>=?)
		GROUP BY `+column+` ORDER BY COUNT(*) DESC,`+column+` LIMIT 10`, campaignID, since, since)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MetadataCount
	for rows.Next() {
		var item MetadataCount
		if err := rows.Scan(&item.Value, &item.Count); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (q *Querier) fieldSummaries(ctx context.Context, campaignID int64, start *time.Time) ([]FieldSummary, error) {
	fields, err := q.ListFormFields(ctx, campaignID, true)
	if err != nil {
		return nil, err
	}
	fieldMap := make(map[string]FormField, len(fields))
	for _, field := range fields {
		fieldMap[field.PublicID] = field
	}
	since := ""
	if start != nil {
		since = start.UTC().Format(time.RFC3339Nano)
	}
	var submissionCount int64
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_submissions WHERE campaign_id=? AND (?='' OR submitted_at>=?)`, campaignID, since, since).Scan(&submissionCount); err != nil {
		return nil, err
	}
	rows, err := q.db.QueryContext(ctx, `SELECT a.field_public_id,a.field_type,a.field_label_snapshot,a.value_json
		FROM campaign_submission_answers a JOIN campaign_submissions s ON s.id=a.submission_id
		WHERE s.campaign_id=? AND (?='' OR s.submitted_at>=?) ORDER BY a.id`, campaignID, since, since)
	if err != nil {
		return nil, err
	}
	type accumulator struct {
		summary FieldSummary
		counts  map[string]int64
		sum     float64
	}
	accumulators := map[string]*accumulator{}
	order := []string{}
	for rows.Next() {
		var publicID, fieldType, label, raw string
		if err := rows.Scan(&publicID, &fieldType, &label, &raw); err != nil {
			rows.Close()
			return nil, err
		}
		acc := accumulators[publicID]
		if acc == nil {
			field, exists := fieldMap[publicID]
			acc = &accumulator{summary: FieldSummary{PublicID: publicID, FieldType: fieldType, Label: label, Archived: !exists || field.ArchivedAt.Valid}, counts: map[string]int64{}}
			accumulators[publicID] = acc
			order = append(order, publicID)
		}
		acc.summary.Answered++
		var value any
		if json.Unmarshal([]byte(raw), &value) != nil {
			continue
		}
		switch typed := value.(type) {
		case float64:
			key := fmt.Sprint(int(typed))
			acc.counts[key]++
			acc.sum += typed
		case string:
			if acc.summary.FieldType != "textarea" {
				acc.counts[typed]++
			}
		case []any:
			for _, item := range typed {
				acc.counts[fmt.Sprint(item)]++
				acc.summary.TotalSelections++
			}
		}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for _, field := range fields {
		if field.FieldType == "text_block" {
			continue
		}
		if accumulators[field.PublicID] == nil && !field.ArchivedAt.Valid {
			accumulators[field.PublicID] = &accumulator{summary: FieldSummary{PublicID: field.PublicID, FieldType: field.FieldType, Label: field.Label}, counts: map[string]int64{}}
			order = append(order, field.PublicID)
		}
	}
	var summaries []FieldSummary
	for _, publicID := range order {
		acc := accumulators[publicID]
		acc.summary.Skipped = submissionCount - acc.summary.Answered
		if acc.summary.FieldType == "rating_1_5" && acc.summary.Answered > 0 {
			acc.summary.Average = acc.sum / float64(acc.summary.Answered)
		}
		field := fieldMap[publicID]
		labels := map[string]string{}
		for _, option := range field.Options {
			labels[option.Value] = option.Label
			if _, exists := acc.counts[option.Value]; !exists {
				acc.counts[option.Value] = 0
			}
		}
		if acc.summary.FieldType == "rating_1_5" {
			for rating := 1; rating <= 5; rating++ {
				key := fmt.Sprint(rating)
				if _, exists := acc.counts[key]; !exists {
					acc.counts[key] = 0
				}
			}
		}
		keys := make([]string, 0, len(acc.counts))
		for key := range acc.counts {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			denominator := acc.summary.Answered
			if acc.summary.FieldType == "checkbox_group" {
				denominator = submissionCount
			}
			percentage := 0.0
			if denominator > 0 {
				percentage = float64(acc.counts[key]) * 100 / float64(denominator)
			}
			label := labels[key]
			if label == "" {
				label = key
			}
			acc.summary.Values = append(acc.summary.Values, ValueCount{Value: key, Label: label, Count: acc.counts[key], Percentage: percentage})
		}
		summaries = append(summaries, acc.summary)
	}
	return summaries, rows.Err()
}

func (q *Querier) ListSubmissionsWithAnswers(ctx context.Context, campaignID int64) ([]Submission, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT s.id, s.public_id, s.campaign_id, v.public_id, s.install_token_hash IS NOT NULL, s.submitted_at, v.context_json,
		       a.field_public_id, a.field_type, a.field_label_snapshot, a.value_json
		FROM campaign_submissions s
		LEFT JOIN campaign_visits v ON v.id = s.visit_id
		LEFT JOIN campaign_submission_answers a ON a.submission_id = s.id
		WHERE s.campaign_id = ?
		ORDER BY s.id DESC, a.id ASC
	`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var submissions []Submission
	submissionMap := make(map[int64]int) // maps submission ID to index in submissions slice

	for rows.Next() {
		var sID, sCampaignID int64
		var sPublicID, sSubmittedAt string
		var sVisitPublicID sql.NullString
		var sHasInstallTokenHash bool
		var contextJSON sql.NullString

		var aFieldPublicID, aFieldType, aFieldLabelSnapshot, aValueJSON sql.NullString

		err := rows.Scan(
			&sID, &sPublicID, &sCampaignID, &sVisitPublicID, &sHasInstallTokenHash, &sSubmittedAt, &contextJSON,
			&aFieldPublicID, &aFieldType, &aFieldLabelSnapshot, &aValueJSON,
		)
		if err != nil {
			return nil, err
		}

		idx, exists := submissionMap[sID]
		if !exists {
			var urlContext map[string]string
			if contextJSON.Valid {
				_ = json.Unmarshal([]byte(contextJSON.String), &urlContext)
			}
			sub := Submission{
				ID:                  sID,
				PublicID:            sPublicID,
				CampaignID:          sCampaignID,
				VisitPublicID:       sVisitPublicID,
				HasInstallTokenHash: sHasInstallTokenHash,
				SubmittedAt:         sSubmittedAt,
				URLContext:          urlContext,
			}
			submissions = append(submissions, sub)
			idx = len(submissions) - 1
			submissionMap[sID] = idx
		}

		if aFieldPublicID.Valid {
			submissions[idx].Answers = append(submissions[idx].Answers, SubmissionAnswer{
				FieldPublicID:      aFieldPublicID.String,
				FieldType:          aFieldType.String,
				FieldLabelSnapshot: aFieldLabelSnapshot.String,
				ValueJSON:          aValueJSON.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return submissions, nil
}

func (q *Querier) AuditCampaignExport(ctx context.Context, actorID int64, campaign Campaign, format string, count int) error {
	metadata, _ := json.Marshal(map[string]any{"format": format, "campaign_public_id": campaign.PublicID, "approximate_submission_count": count})
	return q.CreateAuditEvent(ctx, actorID, campaign.OrganizationID, "campaign.export."+format, "campaign", campaign.PublicID, nil, string(metadata))
}

type DeletionCounts struct {
	Visits, Submissions int64
}

func (q *Querier) DeleteOldCampaignData(ctx context.Context, campaign Campaign, cutoff time.Time, actorID int64) (DeletionCounts, error) {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return DeletionCounts{}, err
	}
	defer tx.Rollback()
	cutoffText := cutoff.UTC().Format(time.RFC3339Nano)
	submissions, err := tx.ExecContext(ctx, `DELETE FROM campaign_submissions WHERE campaign_id=? AND submitted_at<?`, campaign.ID, cutoffText)
	if err != nil {
		return DeletionCounts{}, err
	}
	visits, err := tx.ExecContext(ctx, `DELETE FROM campaign_visits WHERE campaign_id=? AND created_at<?`, campaign.ID, cutoffText)
	if err != nil {
		return DeletionCounts{}, err
	}
	submissionCount, _ := submissions.RowsAffected()
	visitCount, _ := visits.RowsAffected()
	metadata, _ := json.Marshal(map[string]any{"cutoff": cutoffText, "deleted_submissions": submissionCount, "deleted_visits": visitCount})
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id,metadata_json,created_at) VALUES(?,?,'campaign.retention.delete_old','campaign',?,?,?)`, actorID, campaign.OrganizationID, campaign.PublicID, string(metadata), Now()); err != nil {
		return DeletionCounts{}, err
	}
	return DeletionCounts{Visits: visitCount, Submissions: submissionCount}, tx.Commit()
}

func (q *Querier) DeleteAllCampaignResponses(ctx context.Context, campaign Campaign, actorID int64) (int64, error) {
	return q.deleteAllCampaignRows(ctx, campaign, actorID, "campaign.responses.delete_all", "campaign_submissions")
}

func (q *Querier) DeleteAllCampaignVisits(ctx context.Context, campaign Campaign, actorID int64) (int64, error) {
	return q.deleteAllCampaignRows(ctx, campaign, actorID, "campaign.visits.delete_all", "campaign_visits")
}

func (q *Querier) deleteAllCampaignRows(ctx context.Context, campaign Campaign, actorID int64, action, table string) (int64, error) {
	allowed := map[string]bool{"campaign_submissions": true, "campaign_visits": true}
	if !allowed[table] {
		return 0, ErrForbidden
	}
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `DELETE FROM `+table+` WHERE campaign_id=?`, campaign.ID)
	if err != nil {
		return 0, err
	}
	count, _ := result.RowsAffected()
	metadata, _ := json.Marshal(map[string]any{"deleted_count": count})
	if _, err = tx.ExecContext(ctx, `INSERT INTO audit_log(actor_user_id,organization_id,action,target_type,target_id,metadata_json,created_at) VALUES(?,?,?,?,?,?,?)`, actorID, campaign.OrganizationID, action, "campaign", campaign.PublicID, string(metadata), Now()); err != nil {
		return 0, err
	}
	return count, tx.Commit()
}
