package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var ErrSubmissionLimitReached = errors.New("monthly submission safety limit reached")

type FormField struct {
	ID         int64
	PublicID   string
	CampaignID int64
	FieldType  string
	Label      string
	HelpText   sql.NullString
	Required   bool
	SortOrder  int
	ConfigJSON sql.NullString
	CreatedAt  string
	UpdatedAt  string
	ArchivedAt sql.NullString
	Options    []FormOption
}

type FormOption struct {
	ID         int64
	PublicID   string
	FieldID    int64
	Label      string
	Value      string
	SortOrder  int
	CreatedAt  string
	UpdatedAt  string
	ArchivedAt sql.NullString
}

type FieldConfig struct {
	Body      string `json:"body,omitempty"`
	MaxLength int    `json:"max_length,omitempty"`
}

func (f FormField) Config() FieldConfig {
	var config FieldConfig
	if f.ConfigJSON.Valid {
		_ = json.Unmarshal([]byte(f.ConfigJSON.String), &config)
	}
	if f.FieldType == "textarea" && config.MaxLength == 0 {
		config.MaxLength = 1000
	}
	return config
}

type SaveFormFieldInput struct {
	PublicID, FieldType, Label, HelpText, ConfigJSON string
	CampaignID                                       int64
	Required                                         bool
}

func (q *Querier) ListFormFields(ctx context.Context, campaignID int64, includeArchived bool) ([]FormField, error) {
	archived := "AND archived_at IS NULL"
	if includeArchived {
		archived = ""
	}
	rows, err := q.db.QueryContext(ctx, `SELECT id,public_id,campaign_id,field_type,label,help_text,required,sort_order,config_json,created_at,updated_at,archived_at
		FROM campaign_form_fields WHERE campaign_id=? `+archived+` ORDER BY sort_order,id`, campaignID)
	if err != nil {
		return nil, err
	}
	var fields []FormField
	for rows.Next() {
		var field FormField
		if err := rows.Scan(&field.ID, &field.PublicID, &field.CampaignID, &field.FieldType, &field.Label, &field.HelpText, &field.Required, &field.SortOrder, &field.ConfigJSON, &field.CreatedAt, &field.UpdatedAt, &field.ArchivedAt); err != nil {
			rows.Close()
			return nil, err
		}
		fields = append(fields, field)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range fields {
		fields[index].Options, err = q.listFormOptions(ctx, fields[index].ID, includeArchived)
		if err != nil {
			return nil, err
		}
	}
	return fields, nil
}

func (q *Querier) GetFormField(ctx context.Context, campaignID int64, publicID string) (FormField, error) {
	var field FormField
	err := q.db.QueryRowContext(ctx, `SELECT id,public_id,campaign_id,field_type,label,help_text,required,sort_order,config_json,created_at,updated_at,archived_at
		FROM campaign_form_fields WHERE campaign_id=? AND public_id=?`, campaignID, publicID).
		Scan(&field.ID, &field.PublicID, &field.CampaignID, &field.FieldType, &field.Label, &field.HelpText, &field.Required, &field.SortOrder, &field.ConfigJSON, &field.CreatedAt, &field.UpdatedAt, &field.ArchivedAt)
	if err != nil {
		return field, err
	}
	field.Options, err = q.listFormOptions(ctx, field.ID, true)
	return field, err
}

func (q *Querier) listFormOptions(ctx context.Context, fieldID int64, includeArchived bool) ([]FormOption, error) {
	archived := "AND archived_at IS NULL"
	if includeArchived {
		archived = ""
	}
	rows, err := q.db.QueryContext(ctx, `SELECT id,public_id,field_id,label,value,sort_order,created_at,updated_at,archived_at
		FROM campaign_form_options WHERE field_id=? `+archived+` ORDER BY sort_order,id`, fieldID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var options []FormOption
	for rows.Next() {
		var option FormOption
		if err := rows.Scan(&option.ID, &option.PublicID, &option.FieldID, &option.Label, &option.Value, &option.SortOrder, &option.CreatedAt, &option.UpdatedAt, &option.ArchivedAt); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	return options, rows.Err()
}

func (q *Querier) CreateFormField(ctx context.Context, input SaveFormFieldInput, actorID int64) error {
	now := Now()
	_, err := q.db.ExecContext(ctx, `INSERT INTO campaign_form_fields(public_id,campaign_id,field_type,label,help_text,required,sort_order,config_json,created_at,updated_at)
		VALUES(?,?,?,?,?,?,(SELECT COALESCE(MAX(sort_order),0)+1 FROM campaign_form_fields WHERE campaign_id=?),?,?,?)`,
		input.PublicID, input.CampaignID, input.FieldType, input.Label, nullableText(input.HelpText), input.Required, input.CampaignID, nullableText(input.ConfigJSON), now, now)
	if err != nil {
		return err
	}
	return q.auditFormChange(ctx, input.CampaignID, actorID, "campaign_form_field_created", input.PublicID)
}

func (q *Querier) UpdateFormField(ctx context.Context, campaignID int64, publicID string, input SaveFormFieldInput, actorID int64) error {
	res, err := q.db.ExecContext(ctx, `UPDATE campaign_form_fields SET label=?,help_text=?,required=?,config_json=?,updated_at=?
		WHERE campaign_id=? AND public_id=? AND archived_at IS NULL`,
		input.Label, nullableText(input.HelpText), input.Required, nullableText(input.ConfigJSON), Now(), campaignID, publicID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return q.auditFormChange(ctx, campaignID, actorID, "campaign_form_field_updated", publicID)
}

func (q *Querier) ArchiveFormField(ctx context.Context, campaignID int64, publicID string, actorID int64) error {
	res, err := q.db.ExecContext(ctx, `UPDATE campaign_form_fields SET archived_at=?,updated_at=? WHERE campaign_id=? AND public_id=? AND archived_at IS NULL`, Now(), Now(), campaignID, publicID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return q.auditFormChange(ctx, campaignID, actorID, "campaign_form_field_archived", publicID)
}

func (q *Querier) MoveFormField(ctx context.Context, campaignID int64, publicID, direction string, actorID int64) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	var order int
	if err = tx.QueryRowContext(ctx, `SELECT id,sort_order FROM campaign_form_fields WHERE campaign_id=? AND public_id=? AND archived_at IS NULL`, campaignID, publicID).Scan(&id, &order); err != nil {
		return err
	}
	operator, ordering := "<", "DESC"
	if direction == "down" {
		operator, ordering = ">", "ASC"
	}
	var otherID int64
	var otherOrder int
	err = tx.QueryRowContext(ctx, `SELECT id,sort_order FROM campaign_form_fields WHERE campaign_id=? AND archived_at IS NULL AND sort_order `+operator+` ? ORDER BY sort_order `+ordering+` LIMIT 1`, campaignID, order).Scan(&otherID, &otherOrder)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE campaign_form_fields SET sort_order=CASE id WHEN ? THEN ? WHEN ? THEN ? END,updated_at=? WHERE id IN (?,?)`, id, otherOrder, otherID, order, Now(), id, otherID); err != nil {
		return err
	}
	return tx.Commit()
}

func (q *Querier) CreateFormOption(ctx context.Context, campaignID int64, fieldID int64, publicID, label, value string, actorID int64) error {
	now := Now()
	_, err := q.db.ExecContext(ctx, `INSERT INTO campaign_form_options(public_id,field_id,label,value,sort_order,created_at,updated_at)
		VALUES(?,?,?,?,(SELECT COALESCE(MAX(sort_order),0)+1 FROM campaign_form_options WHERE field_id=?),?,?)`, publicID, fieldID, label, value, fieldID, now, now)
	if err != nil {
		return err
	}
	return q.auditFormChange(ctx, campaignID, actorID, "campaign_form_option_created", publicID)
}

func (q *Querier) UpdateFormOption(ctx context.Context, campaignID, fieldID int64, publicID, label string, actorID int64) error {
	res, err := q.db.ExecContext(ctx, `UPDATE campaign_form_options SET label=?,updated_at=? WHERE field_id=? AND public_id=? AND archived_at IS NULL`, label, Now(), fieldID, publicID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return q.auditFormChange(ctx, campaignID, actorID, "campaign_form_option_updated", publicID)
}

func (q *Querier) ArchiveFormOption(ctx context.Context, campaignID int64, publicID string, actorID int64) error {
	res, err := q.db.ExecContext(ctx, `UPDATE campaign_form_options SET archived_at=?,updated_at=? WHERE public_id=? AND field_id IN (SELECT id FROM campaign_form_fields WHERE campaign_id=?) AND archived_at IS NULL`, Now(), Now(), publicID, campaignID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return q.auditFormChange(ctx, campaignID, actorID, "campaign_form_option_archived", publicID)
}

func (q *Querier) auditFormChange(ctx context.Context, campaignID, actorID int64, action, targetID string) error {
	var orgID int64
	if err := q.db.QueryRowContext(ctx, `SELECT organization_id FROM campaigns WHERE id=?`, campaignID).Scan(&orgID); err != nil {
		return err
	}
	return q.CreateAuditEvent(ctx, actorID, orgID, action, "campaign_form", targetID, nil, nil)
}

type SubmissionAnswerInput struct {
	FieldID            int64
	FieldPublicID      string
	FieldType          string
	FieldLabelSnapshot string
	ValueJSON          string
}

type CreateSubmissionInput struct {
	PublicID, VisitPublicID string
	CampaignID, OrgID       int64
	SubmittedAt             time.Time
	Answers                 []SubmissionAnswerInput
}

func (q *Querier) CreateSubmission(ctx context.Context, input CreateSubmissionInput) error {
	tx, err := q.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	at := input.SubmittedAt.UTC()
	start := time.Date(at.Year(), at.Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	var limit, count int64
	if err = tx.QueryRowContext(ctx, `SELECT max_monthly_submissions FROM organization_limits WHERE organization_id=?`, input.OrgID).Scan(&limit); err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_submissions s JOIN campaigns c ON c.id=s.campaign_id
		WHERE c.organization_id=? AND s.submitted_at>=? AND s.submitted_at<?`, input.OrgID, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano)).Scan(&count); err != nil {
		return err
	}
	if count >= limit {
		return ErrSubmissionLimitReached
	}
	var visitID sql.NullInt64
	var tokenHash sql.NullString
	if input.VisitPublicID != "" {
		_ = tx.QueryRowContext(ctx, `SELECT id,install_token_hash FROM campaign_visits WHERE public_id=? AND campaign_id=?`, input.VisitPublicID, input.CampaignID).Scan(&visitID, &tokenHash)
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO campaign_submissions(public_id,campaign_id,visit_id,install_token_hash,submitted_at) VALUES(?,?,?,?,?)`,
		input.PublicID, input.CampaignID, nullableInt64(visitID), nullableText(tokenHash.String), at.Format(time.RFC3339Nano))
	if err != nil {
		return err
	}
	submissionID, _ := result.LastInsertId()
	for _, answer := range input.Answers {
		if _, err = tx.ExecContext(ctx, `INSERT INTO campaign_submission_answers(submission_id,field_id,field_public_id,field_type,field_label_snapshot,value_json)
			VALUES(?,?,?,?,?,?)`, submissionID, answer.FieldID, answer.FieldPublicID, answer.FieldType, answer.FieldLabelSnapshot, answer.ValueJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func nullableInt64(value sql.NullInt64) any {
	if !value.Valid {
		return nil
	}
	return value.Int64
}

type Submission struct {
	ID                  int64
	PublicID            string
	CampaignID          int64
	VisitPublicID       sql.NullString
	HasInstallTokenHash bool
	SubmittedAt         string
	AnswerSummary       string
	Answers             []SubmissionAnswer
}

type SubmissionAnswer struct {
	FieldPublicID, FieldType, FieldLabelSnapshot, ValueJSON string
}

type SubmissionStats struct {
	Total, CurrentMonth int64
	LatestAt            sql.NullString
}

func (q *Querier) SubmissionStats(ctx context.Context, campaignID int64, now time.Time) (SubmissionStats, error) {
	start := time.Date(now.UTC().Year(), now.UTC().Month(), 1, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 1, 0)
	var stats SubmissionStats
	err := q.db.QueryRowContext(ctx, `SELECT COUNT(*),COALESCE(SUM(CASE WHEN submitted_at>=? AND submitted_at<? THEN 1 ELSE 0 END),0),MAX(submitted_at)
		FROM campaign_submissions WHERE campaign_id=?`, start.Format(time.RFC3339Nano), end.Format(time.RFC3339Nano), campaignID).
		Scan(&stats.Total, &stats.CurrentMonth, &stats.LatestAt)
	return stats, err
}

func (q *Querier) ListSubmissions(ctx context.Context, campaignID int64) ([]Submission, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT s.id,s.public_id,s.campaign_id,v.public_id,s.install_token_hash IS NOT NULL,s.submitted_at,
		COALESCE((SELECT a.field_label_snapshot FROM campaign_submission_answers a WHERE a.submission_id=s.id ORDER BY a.id LIMIT 1),'')
		FROM campaign_submissions s LEFT JOIN campaign_visits v ON v.id=s.visit_id WHERE s.campaign_id=? ORDER BY s.id DESC LIMIT 100`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var submissions []Submission
	for rows.Next() {
		var submission Submission
		if err := rows.Scan(&submission.ID, &submission.PublicID, &submission.CampaignID, &submission.VisitPublicID, &submission.HasInstallTokenHash, &submission.SubmittedAt, &submission.AnswerSummary); err != nil {
			return nil, err
		}
		submissions = append(submissions, submission)
	}
	return submissions, rows.Err()
}

func (q *Querier) GetSubmission(ctx context.Context, campaignID int64, publicID string) (Submission, error) {
	var submission Submission
	err := q.db.QueryRowContext(ctx, `SELECT s.id,s.public_id,s.campaign_id,v.public_id,s.install_token_hash IS NOT NULL,s.submitted_at,''
		FROM campaign_submissions s LEFT JOIN campaign_visits v ON v.id=s.visit_id WHERE s.campaign_id=? AND s.public_id=?`, campaignID, publicID).
		Scan(&submission.ID, &submission.PublicID, &submission.CampaignID, &submission.VisitPublicID, &submission.HasInstallTokenHash, &submission.SubmittedAt, &submission.AnswerSummary)
	if err != nil {
		return submission, err
	}
	rows, err := q.db.QueryContext(ctx, `SELECT field_public_id,field_type,field_label_snapshot,value_json FROM campaign_submission_answers WHERE submission_id=? ORDER BY id`, submission.ID)
	if err != nil {
		return submission, err
	}
	defer rows.Close()
	for rows.Next() {
		var answer SubmissionAnswer
		if err := rows.Scan(&answer.FieldPublicID, &answer.FieldType, &answer.FieldLabelSnapshot, &answer.ValueJSON); err != nil {
			return submission, err
		}
		submission.Answers = append(submission.Answers, answer)
	}
	return submission, rows.Err()
}
