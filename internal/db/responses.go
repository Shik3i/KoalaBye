package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	ResponsePageSize     = 50
	maxBulkSelectedIDs   = 200
	maxSearchTerms       = 5
	maxSearchTermRunes   = 100
	readExistsAny        = `EXISTS (SELECT 1 FROM campaign_submission_reads r WHERE r.submission_id = s.id)`
	readExistsMine       = `EXISTS (SELECT 1 FROM campaign_submission_reads r WHERE r.submission_id = s.id AND r.user_id = ?)`
	readExistsOthers     = `EXISTS (SELECT 1 FROM campaign_submission_reads r WHERE r.submission_id = s.id AND r.user_id <> ?)`
	bulkActionRead       = "read"
	bulkActionUnread     = "unread"
	bulkActionStar       = "star"
	bulkActionUnstar     = "unstar"
	bulkActionStatusPref = "status_"
)

var triageStatuses = []string{"new", "reviewed", "actionable", "closed"}

// ResponseFilter describes the list view of a campaign's responses.
type ResponseFilter struct {
	Status  string // "", new, reviewed, actionable, closed, open
	Read    string // "", unread, unread_me, team, mine
	Content string // "", text, choices
	Starred bool
	Query   string
	From    string // YYYY-MM-DD
	To      string // YYYY-MM-DD
	Oldest  bool
	Page    int
}

// Normalize drops unknown values so the filter is always safe to render and query.
func (f ResponseFilter) Normalize() ResponseFilter {
	switch f.Status {
	case "new", "reviewed", "actionable", "closed", "open":
	default:
		f.Status = ""
	}
	switch f.Read {
	case "unread", "unread_me", "team", "mine":
	default:
		f.Read = ""
	}
	switch f.Content {
	case "text", "choices":
	default:
		f.Content = ""
	}
	f.Query = strings.Join(searchTerms(f.Query), " ")
	if _, err := time.Parse("2006-01-02", f.From); err != nil {
		f.From = ""
	}
	if _, err := time.Parse("2006-01-02", f.To); err != nil {
		f.To = ""
	}
	if f.Page < 1 {
		f.Page = 1
	}
	return f
}

// Values encodes the filter as query parameters (page is only set when above 1).
func (f ResponseFilter) Values() url.Values {
	values := url.Values{}
	if f.Status != "" {
		values.Set("status", f.Status)
	}
	if f.Read != "" {
		values.Set("read", f.Read)
	}
	if f.Content != "" {
		values.Set("content", f.Content)
	}
	if f.Starred {
		values.Set("starred", "1")
	}
	if f.Query != "" {
		values.Set("q", f.Query)
	}
	if f.From != "" {
		values.Set("from", f.From)
	}
	if f.To != "" {
		values.Set("to", f.To)
	}
	if f.Oldest {
		values.Set("sort", "oldest")
	}
	if f.Page > 1 {
		values.Set("page", strconv.Itoa(f.Page))
	}
	return values
}

// IsFiltered reports whether any narrowing criterion is active.
func (f ResponseFilter) IsFiltered() bool {
	return f.Status != "" || f.Read != "" || f.Content != "" || f.Starred || f.Query != "" || f.From != "" || f.To != ""
}

func (f ResponseFilter) where(campaignID, userID int64) (string, []any) {
	clauses := []string{"s.campaign_id = ?"}
	args := []any{campaignID}
	switch f.Status {
	case "":
	case "open":
		clauses = append(clauses, "s.triage_status <> 'closed'")
	default:
		clauses = append(clauses, "s.triage_status = ?")
		args = append(args, f.Status)
	}
	switch f.Read {
	case "unread":
		clauses = append(clauses, "NOT "+readExistsAny)
	case "unread_me":
		clauses = append(clauses, "NOT "+readExistsMine)
		args = append(args, userID)
	case "mine":
		clauses = append(clauses, readExistsMine)
		args = append(args, userID)
	case "team":
		clauses = append(clauses, readExistsOthers)
		args = append(args, userID)
	}
	switch f.Content {
	case "text":
		clauses = append(clauses, "s.has_text = 1")
	case "choices":
		clauses = append(clauses, "s.has_text = 0")
	}
	if f.Starred {
		clauses = append(clauses, "s.starred = 1")
	}
	for _, term := range searchTerms(f.Query) {
		clauses = append(clauses, `(s.public_id = ? OR EXISTS (SELECT 1 FROM campaign_submission_answers a WHERE a.submission_id = s.id AND a.value_json LIKE ? ESCAPE '\'))`)
		args = append(args, term, "%"+escapeLike(term)+"%")
	}
	if from, err := time.Parse("2006-01-02", f.From); err == nil {
		clauses = append(clauses, "s.submitted_at >= ?")
		args = append(args, from.UTC().Format(time.RFC3339Nano))
	}
	if to, err := time.Parse("2006-01-02", f.To); err == nil {
		clauses = append(clauses, "s.submitted_at < ?")
		args = append(args, to.AddDate(0, 0, 1).UTC().Format(time.RFC3339Nano))
	}
	return strings.Join(clauses, " AND "), args
}

func searchTerms(query string) []string {
	var terms []string
	for _, term := range strings.Fields(query) {
		runes := []rune(term)
		if len(runes) > maxSearchTermRunes {
			term = string(runes[:maxSearchTermRunes])
		}
		terms = append(terms, term)
		if len(terms) == maxSearchTerms {
			break
		}
	}
	return terms
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// ResponseCounts are campaign-wide totals, independent of the active filter.
type ResponseCounts struct {
	Total, Unread, UnreadMe, WithText, Starred, Actionable int64
}

type ResponsePage struct {
	Submissions []Submission
	Matching    int64
	Page        int
	PageCount   int
	Counts      ResponseCounts
}

type SubmissionReader struct {
	UserPublicID string
	Name         string
	FirstReadAt  string
	IsMe         bool
}

type SubmissionNeighbors struct {
	NewerPublicID, OlderPublicID, NextUnreadPublicID string
}

func (q *Querier) ResponseCounts(ctx context.Context, campaignID, userID int64) (ResponseCounts, error) {
	var counts ResponseCounts
	err := q.db.QueryRowContext(ctx, `SELECT COUNT(*),
			COALESCE(SUM(CASE WHEN NOT `+readExistsAny+` THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN NOT `+readExistsMine+` THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(s.has_text), 0),
			COALESCE(SUM(s.starred), 0),
			COALESCE(SUM(CASE WHEN s.triage_status = 'actionable' THEN 1 ELSE 0 END), 0)
		FROM campaign_submissions s WHERE s.campaign_id = ?`, userID, campaignID).
		Scan(&counts.Total, &counts.Unread, &counts.UnreadMe, &counts.WithText, &counts.Starred, &counts.Actionable)
	return counts, err
}

// UnreadSubmissionCount returns how many responses the user has not opened yet.
func (q *Querier) UnreadSubmissionCount(ctx context.Context, campaignID, userID int64) (int64, error) {
	var count int64
	err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_submissions s WHERE s.campaign_id = ? AND NOT `+readExistsMine,
		campaignID, userID).Scan(&count)
	return count, err
}

func (q *Querier) ListSubmissionsPage(ctx context.Context, campaignID, userID int64, filter ResponseFilter) (ResponsePage, error) {
	filter = filter.Normalize()
	page := ResponsePage{Page: filter.Page}
	where, args := filter.where(campaignID, userID)
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_submissions s WHERE `+where, args...).Scan(&page.Matching); err != nil {
		return page, err
	}
	page.PageCount = int((page.Matching + ResponsePageSize - 1) / ResponsePageSize)
	if page.PageCount < 1 {
		page.PageCount = 1
	}
	if page.Page > page.PageCount {
		page.Page = page.PageCount
	}
	order := "DESC"
	if filter.Oldest {
		order = "ASC"
	}
	rowArgs := append([]any{userID, userID}, args...)
	rowArgs = append(rowArgs, ResponsePageSize, (page.Page-1)*ResponsePageSize)
	rows, err := q.db.QueryContext(ctx, `SELECT s.id, s.public_id, s.campaign_id, v.public_id, s.install_token_hash IS NOT NULL, s.submitted_at, s.triage_status,
			s.has_text, s.starred, `+readExistsMine+`, (SELECT COUNT(*) FROM campaign_submission_reads r WHERE r.submission_id = s.id AND r.user_id <> ?)
		FROM campaign_submissions s LEFT JOIN campaign_visits v ON v.id = s.visit_id
		WHERE `+where+` ORDER BY s.id `+order+` LIMIT ? OFFSET ?`, rowArgs...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	index := map[int64]int{}
	var ids []any
	for rows.Next() {
		var submission Submission
		if err := rows.Scan(&submission.ID, &submission.PublicID, &submission.CampaignID, &submission.VisitPublicID, &submission.HasInstallTokenHash,
			&submission.SubmittedAt, &submission.TriageStatus, &submission.HasText, &submission.Starred, &submission.ReadByMe, &submission.ReadByOthers); err != nil {
			return page, err
		}
		index[submission.ID] = len(page.Submissions)
		ids = append(ids, submission.ID)
		page.Submissions = append(page.Submissions, submission)
	}
	if err := rows.Err(); err != nil {
		return page, err
	}
	if len(ids) > 0 {
		answerRows, err := q.db.QueryContext(ctx, `SELECT submission_id, field_id, field_public_id, field_type, field_label_snapshot, value_json
			FROM campaign_submission_answers WHERE submission_id IN (`+placeholders(len(ids))+`) ORDER BY id`, ids...)
		if err != nil {
			return page, err
		}
		defer answerRows.Close()
		for answerRows.Next() {
			var submissionID int64
			var fieldID sql.NullInt64
			var answer SubmissionAnswer
			if err := answerRows.Scan(&submissionID, &fieldID, &answer.FieldPublicID, &answer.FieldType, &answer.FieldLabelSnapshot, &answer.ValueJSON); err != nil {
				return page, err
			}
			answer.FieldID = fieldID.Int64
			position := index[submissionID]
			page.Submissions[position].Answers = append(page.Submissions[position].Answers, answer)
		}
		if err := answerRows.Err(); err != nil {
			return page, err
		}
		labels, err := q.campaignOptionLabels(ctx, campaignID)
		if err != nil {
			return page, err
		}
		for i := range page.Submissions {
			applyDisplayLabels(&page.Submissions[i], labels)
		}
	}
	page.Counts, err = q.ResponseCounts(ctx, campaignID, userID)
	return page, err
}

func placeholders(count int) string {
	if count < 1 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", count), ",")
}

// MarkSubmissionRead records the first time a user opened a response.
func (q *Querier) MarkSubmissionRead(ctx context.Context, submissionID, userID int64) error {
	_, err := q.db.ExecContext(ctx, `INSERT OR IGNORE INTO campaign_submission_reads(submission_id, user_id, first_read_at) VALUES(?,?,?)`,
		submissionID, userID, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (q *Querier) ListSubmissionReaders(ctx context.Context, submissionID, userID int64) ([]SubmissionReader, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT u.public_id, CASE WHEN u.display_name <> '' THEN u.display_name ELSE u.username END, r.first_read_at, u.id = ?
		FROM campaign_submission_reads r JOIN users u ON u.id = r.user_id WHERE r.submission_id = ? ORDER BY r.first_read_at`, userID, submissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var readers []SubmissionReader
	for rows.Next() {
		var reader SubmissionReader
		if err := rows.Scan(&reader.UserPublicID, &reader.Name, &reader.FirstReadAt, &reader.IsMe); err != nil {
			return nil, err
		}
		readers = append(readers, reader)
	}
	return readers, rows.Err()
}

func (q *Querier) GetSubmissionNeighbors(ctx context.Context, campaignID, submissionID, userID int64) (SubmissionNeighbors, error) {
	var neighbors SubmissionNeighbors
	lookup := func(target *string, query string, args ...any) error {
		var publicID string
		err := q.db.QueryRowContext(ctx, query, args...).Scan(&publicID)
		if err == sql.ErrNoRows {
			return nil
		}
		*target = publicID
		return err
	}
	if err := lookup(&neighbors.NewerPublicID, `SELECT public_id FROM campaign_submissions WHERE campaign_id = ? AND id > ? ORDER BY id ASC LIMIT 1`, campaignID, submissionID); err != nil {
		return neighbors, err
	}
	if err := lookup(&neighbors.OlderPublicID, `SELECT public_id FROM campaign_submissions WHERE campaign_id = ? AND id < ? ORDER BY id DESC LIMIT 1`, campaignID, submissionID); err != nil {
		return neighbors, err
	}
	err := lookup(&neighbors.NextUnreadPublicID, `SELECT s.public_id FROM campaign_submissions s WHERE s.campaign_id = ? AND s.id <> ? AND NOT `+readExistsMine+`
		ORDER BY s.id < ? DESC, s.id DESC LIMIT 1`, campaignID, submissionID, userID, submissionID)
	return neighbors, err
}

// BulkTarget selects responses either by public IDs or by every response matching a filter.
type BulkTarget struct {
	PublicIDs []string
	Filter    *ResponseFilter
}

func (t BulkTarget) selection(campaignID, userID int64) (string, []any, error) {
	if t.Filter != nil {
		where, args := t.Filter.Normalize().where(campaignID, userID)
		return where, args, nil
	}
	if len(t.PublicIDs) == 0 {
		return "", nil, ErrForbidden
	}
	if len(t.PublicIDs) > maxBulkSelectedIDs {
		return "", nil, ErrForbidden
	}
	args := []any{campaignID}
	for _, id := range t.PublicIDs {
		args = append(args, id)
	}
	return "s.campaign_id = ? AND s.public_id IN (" + placeholders(len(t.PublicIDs)) + ")", args, nil
}

// BulkActionNeedsEditor reports whether the action changes shared state (not just the caller's read marker).
func BulkActionNeedsEditor(action string) bool {
	return action != bulkActionRead && action != bulkActionUnread
}

func validBulkAction(action string) bool {
	switch action {
	case bulkActionRead, bulkActionUnread, bulkActionStar, bulkActionUnstar:
		return true
	}
	if status, ok := strings.CutPrefix(action, bulkActionStatusPref); ok {
		for _, candidate := range triageStatuses {
			if candidate == status {
				return true
			}
		}
	}
	return false
}

// BulkUpdateSubmissions applies one action to many responses and returns the number touched.
func (q *Querier) BulkUpdateSubmissions(ctx context.Context, campaign Campaign, actorID int64, action string, target BulkTarget) (int64, error) {
	if !validBulkAction(action) {
		return 0, ErrForbidden
	}
	where, args, err := target.selection(campaign.ID, actorID)
	if err != nil {
		return 0, err
	}
	selected := `SELECT s.id FROM campaign_submissions s WHERE ` + where
	var result sql.Result
	switch {
	case action == bulkActionRead:
		result, err = q.db.ExecContext(ctx, `INSERT OR IGNORE INTO campaign_submission_reads(submission_id, user_id, first_read_at)
			SELECT s.id, ?, ? FROM campaign_submissions s WHERE `+where,
			append([]any{actorID, time.Now().UTC().Format(time.RFC3339Nano)}, args...)...)
	case action == bulkActionUnread:
		result, err = q.db.ExecContext(ctx, `DELETE FROM campaign_submission_reads WHERE user_id = ? AND submission_id IN (`+selected+`)`,
			append([]any{actorID}, args...)...)
	case action == bulkActionStar || action == bulkActionUnstar:
		flag := 0
		if action == bulkActionStar {
			flag = 1
		}
		result, err = q.db.ExecContext(ctx, `UPDATE campaign_submissions SET starred = ? WHERE id IN (`+selected+`)`, append([]any{flag}, args...)...)
	default:
		status := strings.TrimPrefix(action, bulkActionStatusPref)
		result, err = q.db.ExecContext(ctx, `UPDATE campaign_submissions SET triage_status = ? WHERE id IN (`+selected+`)`, append([]any{status}, args...)...)
	}
	if err != nil {
		return 0, err
	}
	affected, _ := result.RowsAffected()
	if BulkActionNeedsEditor(action) && affected > 0 {
		metadata, _ := json.Marshal(map[string]any{"action": action, "count": affected, "by_filter": target.Filter != nil})
		if err := q.CreateAuditEvent(ctx, actorID, campaign.OrganizationID, "campaign_response_bulk_updated", "campaign", campaign.PublicID, nil, string(metadata)); err != nil {
			return affected, err
		}
	}
	return affected, nil
}

// campaignOptionLabels maps field ID -> option value -> label for every option of a campaign.
func (q *Querier) campaignOptionLabels(ctx context.Context, campaignID int64) (map[int64]map[string]string, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT o.field_id, o.value, o.label FROM campaign_form_options o
		JOIN campaign_form_fields f ON f.id = o.field_id WHERE f.campaign_id = ?`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	labels := map[int64]map[string]string{}
	for rows.Next() {
		var fieldID int64
		var value, label string
		if err := rows.Scan(&fieldID, &value, &label); err != nil {
			return nil, err
		}
		if labels[fieldID] == nil {
			labels[fieldID] = map[string]string{}
		}
		labels[fieldID][value] = label
	}
	return labels, rows.Err()
}

func applyDisplayLabels(submission *Submission, labels map[int64]map[string]string) {
	for index, answer := range submission.Answers {
		fieldLabels := labels[answer.FieldID]
		if len(fieldLabels) == 0 {
			continue
		}
		switch answer.FieldType {
		case "checkbox_group":
			var values []string
			if json.Unmarshal([]byte(answer.ValueJSON), &values) != nil {
				continue
			}
			changed := false
			for valueIndex, value := range values {
				if label, ok := fieldLabels[value]; ok {
					values[valueIndex] = label
					changed = true
				}
			}
			if changed {
				encoded, _ := json.Marshal(values)
				submission.Answers[index].DisplayValueJSON = string(encoded)
			}
		case "radio_group":
			var value string
			if json.Unmarshal([]byte(answer.ValueJSON), &value) != nil {
				continue
			}
			if label, ok := fieldLabels[value]; ok {
				encoded, _ := json.Marshal(label)
				submission.Answers[index].DisplayValueJSON = string(encoded)
			}
		}
	}
}

// answersHaveText reports whether any free-text answer is non-empty.
func answersHaveText(answers []SubmissionAnswerInput) bool {
	for _, answer := range answers {
		if answer.FieldType != "textarea" {
			continue
		}
		var text string
		if json.Unmarshal([]byte(answer.ValueJSON), &text) == nil && strings.TrimSpace(text) != "" {
			return true
		}
	}
	return false
}
