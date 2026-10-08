package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/koalastuff/koalabye/internal/ids"
)

const (
	maxTagsPerCampaign      = 100
	maxNotesPerSubmission   = 100
	maxNoteRunes            = 2000
	maxViewsPerUser         = 25
	maxViewNameRunes        = 60
	maxInsightAnswers       = 5000
	exportMetaChunk         = 500
	maxInsightWords         = 25
	maxInsightDuplicates    = 10
	insightMinWordRunes     = 3
	insightMinDuplicateRune = 5
)

var ErrInvalidInput = errors.New("invalid input")

// ---- tags ------------------------------------------------------------------

type ResponseTag struct {
	PublicID string
	Name     string
	Count    int64
}

// normalizeTagName returns the display name and the case-folded lookup key.
func normalizeTagName(name string) (display, normalized string, ok bool) {
	cleaned := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name)
	display = strings.Join(strings.Fields(cleaned), " ")
	if display == "" {
		return "", "", false
	}
	if runes := []rune(display); len(runes) > maxTagNameRunes {
		display = string(runes[:maxTagNameRunes])
	}
	return display, strings.ToLower(display), true
}

func (q *Querier) getOrCreateTag(ctx context.Context, campaignID int64, name string) (int64, error) {
	display, normalized, ok := normalizeTagName(name)
	if !ok {
		return 0, ErrInvalidInput
	}
	var id int64
	err := q.db.QueryRowContext(ctx, `SELECT id FROM campaign_response_tags WHERE campaign_id = ? AND name_normalized = ?`, campaignID, normalized).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	var count int
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_response_tags WHERE campaign_id = ?`, campaignID).Scan(&count); err != nil {
		return 0, err
	}
	if count >= maxTagsPerCampaign {
		return 0, ErrLimitReached
	}
	publicID, err := ids.New("tag")
	if err != nil {
		return 0, err
	}
	result, err := q.db.ExecContext(ctx, `INSERT INTO campaign_response_tags(public_id, campaign_id, name, name_normalized, created_at) VALUES(?,?,?,?,?)`,
		publicID, campaignID, display, normalized, Now())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (q *Querier) ListResponseTags(ctx context.Context, campaignID int64) ([]ResponseTag, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT t.public_id, t.name, (SELECT COUNT(*) FROM campaign_submission_tags st WHERE st.tag_id = t.id)
		FROM campaign_response_tags t WHERE t.campaign_id = ? ORDER BY t.name_normalized`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tags []ResponseTag
	for rows.Next() {
		var tag ResponseTag
		if err := rows.Scan(&tag.PublicID, &tag.Name, &tag.Count); err != nil {
			return nil, err
		}
		tags = append(tags, tag)
	}
	return tags, rows.Err()
}

func (q *Querier) DeleteResponseTag(ctx context.Context, campaign Campaign, tagPublicID string, actorID int64) error {
	result, err := q.db.ExecContext(ctx, `DELETE FROM campaign_response_tags WHERE campaign_id = ? AND public_id = ?`, campaign.ID, tagPublicID)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return q.CreateAuditEvent(ctx, actorID, campaign.OrganizationID, "campaign_response_tag_deleted", "campaign", campaign.PublicID, nil, nil)
}

// loadSubmissionTags fills Tags for the given submissions.
func (q *Querier) loadSubmissionTags(ctx context.Context, submissions []Submission) error {
	return forEachSubmissionChunk(submissions, func(chunk []Submission, index map[int64]int, idArgs []any) error {
		rows, err := q.db.QueryContext(ctx, `SELECT st.submission_id, t.name FROM campaign_submission_tags st JOIN campaign_response_tags t ON t.id = st.tag_id
			WHERE st.submission_id IN (`+placeholders(len(idArgs))+`) ORDER BY t.name_normalized`, idArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var submissionID int64
			var name string
			if err := rows.Scan(&submissionID, &name); err != nil {
				return err
			}
			chunk[index[submissionID]].Tags = append(chunk[index[submissionID]].Tags, name)
		}
		return rows.Err()
	})
}

// loadSubmissionReadBy fills ReadBy (display names) for the given submissions.
func (q *Querier) loadSubmissionReadBy(ctx context.Context, submissions []Submission) error {
	return forEachSubmissionChunk(submissions, func(chunk []Submission, index map[int64]int, idArgs []any) error {
		rows, err := q.db.QueryContext(ctx, `SELECT r.submission_id, CASE WHEN u.display_name <> '' THEN u.display_name ELSE u.username END
			FROM campaign_submission_reads r JOIN users u ON u.id = r.user_id
			WHERE r.submission_id IN (`+placeholders(len(idArgs))+`) ORDER BY r.first_read_at`, idArgs...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var submissionID int64
			var name string
			if err := rows.Scan(&submissionID, &name); err != nil {
				return err
			}
			chunk[index[submissionID]].ReadBy = append(chunk[index[submissionID]].ReadBy, name)
		}
		return rows.Err()
	})
}

func forEachSubmissionChunk(submissions []Submission, fn func(chunk []Submission, index map[int64]int, idArgs []any) error) error {
	for start := 0; start < len(submissions); start += exportMetaChunk {
		end := start + exportMetaChunk
		if end > len(submissions) {
			end = len(submissions)
		}
		chunk := submissions[start:end]
		index := make(map[int64]int, len(chunk))
		idArgs := make([]any, 0, len(chunk))
		for i, submission := range chunk {
			index[submission.ID] = i
			idArgs = append(idArgs, submission.ID)
		}
		if err := fn(chunk, index, idArgs); err != nil {
			return err
		}
	}
	return nil
}

// ---- assignment ------------------------------------------------------------

type AssigneeOption struct {
	PublicID string
	Name     string
}

func (q *Querier) ListAssignableMembers(ctx context.Context, campaign Campaign) ([]AssigneeOption, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT u.public_id, CASE WHEN u.display_name <> '' THEN u.display_name ELSE u.username END AS name
		FROM users u WHERE u.disabled_at IS NULL AND EXISTS (SELECT 1 FROM organization_members om WHERE om.organization_id = ? AND om.user_id = u.id)
		AND (EXISTS (SELECT 1 FROM organization_members om WHERE om.organization_id = ? AND om.user_id = u.id AND om.role IN ('owner','admin'))
			OR EXISTS (SELECT 1 FROM campaign_members cm WHERE cm.campaign_id = ? AND cm.user_id = u.id AND cm.role IN ('owner','editor','analyst')))
		ORDER BY name COLLATE NOCASE`, campaign.OrganizationID, campaign.OrganizationID, campaign.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var options []AssigneeOption
	for rows.Next() {
		var option AssigneeOption
		if err := rows.Scan(&option.PublicID, &option.Name); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	return options, rows.Err()
}

func (q *Querier) assignableUserID(ctx context.Context, campaign Campaign, userPublicID string) (int64, error) {
	options, err := q.ListAssignableMembers(ctx, campaign)
	if err != nil {
		return 0, err
	}
	for _, option := range options {
		if option.PublicID == userPublicID {
			var id int64
			if err := q.db.QueryRowContext(ctx, `SELECT id FROM users WHERE public_id = ?`, userPublicID).Scan(&id); err != nil {
				return 0, err
			}
			return id, nil
		}
	}
	return 0, ErrForbidden
}

// ---- notes -----------------------------------------------------------------

type SubmissionNote struct {
	PublicID     string
	AuthorUserID sql.NullInt64
	AuthorName   string
	Body         string
	CreatedAt    string
}

func (q *Querier) ListSubmissionNotes(ctx context.Context, submissionID int64) ([]SubmissionNote, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT n.public_id, n.author_user_id, COALESCE(CASE WHEN u.display_name <> '' THEN u.display_name ELSE u.username END, ''), n.body, n.created_at
		FROM campaign_submission_notes n LEFT JOIN users u ON u.id = n.author_user_id WHERE n.submission_id = ? ORDER BY n.id`, submissionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notes []SubmissionNote
	for rows.Next() {
		var note SubmissionNote
		if err := rows.Scan(&note.PublicID, &note.AuthorUserID, &note.AuthorName, &note.Body, &note.CreatedAt); err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}
	return notes, rows.Err()
}

func (q *Querier) AddSubmissionNote(ctx context.Context, campaign Campaign, submissionPublicID string, authorID int64, body string) error {
	body = strings.TrimSpace(body)
	if body == "" || len([]rune(body)) > maxNoteRunes {
		return ErrInvalidInput
	}
	var submissionID int64
	if err := q.db.QueryRowContext(ctx, `SELECT id FROM campaign_submissions WHERE campaign_id = ? AND public_id = ?`, campaign.ID, submissionPublicID).Scan(&submissionID); err != nil {
		return err
	}
	var count int
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_submission_notes WHERE submission_id = ?`, submissionID).Scan(&count); err != nil {
		return err
	}
	if count >= maxNotesPerSubmission {
		return ErrLimitReached
	}
	publicID, err := ids.New("note")
	if err != nil {
		return err
	}
	_, err = q.db.ExecContext(ctx, `INSERT INTO campaign_submission_notes(public_id, submission_id, author_user_id, body, created_at) VALUES(?,?,?,?,?)`,
		publicID, submissionID, authorID, body, Now())
	return err
}

// DeleteSubmissionNote lets authors delete their own notes; moderators may delete any.
func (q *Querier) DeleteSubmissionNote(ctx context.Context, campaign Campaign, notePublicID string, actorID int64, moderator bool) error {
	query := `DELETE FROM campaign_submission_notes WHERE public_id = ? AND submission_id IN (SELECT id FROM campaign_submissions WHERE campaign_id = ?)`
	args := []any{notePublicID, campaign.ID}
	if !moderator {
		query += ` AND author_user_id = ?`
		args = append(args, actorID)
	}
	result, err := q.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ---- saved views -----------------------------------------------------------

type ResponseView struct {
	PublicID  string
	Name      string
	Shared    bool
	IsMine    bool
	OwnerName string
	Filter    ResponseFilter
}

func (q *Querier) ListResponseViews(ctx context.Context, campaignID, userID int64) ([]ResponseView, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT v.public_id, v.name, v.query, v.shared, v.user_id = ?, CASE WHEN u.display_name <> '' THEN u.display_name ELSE u.username END
		FROM campaign_response_views v JOIN users u ON u.id = v.user_id
		WHERE v.campaign_id = ? AND (v.user_id = ? OR v.shared = 1) ORDER BY v.user_id <> ?, v.name COLLATE NOCASE`, userID, campaignID, userID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var views []ResponseView
	for rows.Next() {
		var view ResponseView
		var query string
		if err := rows.Scan(&view.PublicID, &view.Name, &query, &view.Shared, &view.IsMine, &view.OwnerName); err != nil {
			return nil, err
		}
		values, _ := url.ParseQuery(query)
		view.Filter = ResponseFilterFromValues(values)
		view.Filter.Page = 1
		views = append(views, view)
	}
	return views, rows.Err()
}

func (q *Querier) CreateResponseView(ctx context.Context, campaignID, userID int64, name string, filter ResponseFilter, shared bool) error {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || len([]rune(name)) > maxViewNameRunes {
		return ErrInvalidInput
	}
	filter = filter.Normalize()
	filter.Page = 1
	if !filter.IsFiltered() && !filter.Oldest {
		return ErrInvalidInput
	}
	var count int
	if err := q.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_response_views WHERE campaign_id = ? AND user_id = ?`, campaignID, userID).Scan(&count); err != nil {
		return err
	}
	if count >= maxViewsPerUser {
		return ErrLimitReached
	}
	publicID, err := ids.New("view")
	if err != nil {
		return err
	}
	_, err = q.db.ExecContext(ctx, `INSERT INTO campaign_response_views(public_id, campaign_id, user_id, name, query, shared, created_at) VALUES(?,?,?,?,?,?,?)`,
		publicID, campaignID, userID, name, filter.Values().Encode(), shared, Now())
	return err
}

func (q *Querier) DeleteResponseView(ctx context.Context, campaignID int64, viewPublicID string, userID int64, moderator bool) error {
	query := `DELETE FROM campaign_response_views WHERE campaign_id = ? AND public_id = ?`
	args := []any{campaignID, viewPublicID}
	if !moderator {
		query += ` AND user_id = ?`
		args = append(args, userID)
	}
	result, err := q.db.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ---- auto close ------------------------------------------------------------

func (q *Querier) SetAutoCloseDays(ctx context.Context, campaign Campaign, days sql.NullInt64, actorID int64) error {
	if days.Valid && (days.Int64 < 1 || days.Int64 > 3650) {
		return ErrInvalidInput
	}
	var value any
	if days.Valid {
		value = days.Int64
	}
	if _, err := q.db.ExecContext(ctx, `UPDATE campaign_settings SET auto_close_days = ? WHERE campaign_id = ?`, value, campaign.ID); err != nil {
		return err
	}
	metadata, _ := json.Marshal(map[string]any{"days": value})
	return q.CreateAuditEvent(ctx, actorID, campaign.OrganizationID, "campaign_response_auto_close_updated", "campaign", campaign.PublicID, nil, string(metadata))
}

// RunAutoClose closes responses that someone has read and nobody is working on:
// status new/reviewed, not starred, not assigned, older than the campaign's auto_close_days.
func (q *Querier) RunAutoClose(ctx context.Context, now time.Time) (int64, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT c.id, c.public_id, c.organization_id, cs.auto_close_days FROM campaign_settings cs
		JOIN campaigns c ON c.id = cs.campaign_id WHERE cs.auto_close_days IS NOT NULL AND c.status <> 'archived'`)
	if err != nil {
		return 0, err
	}
	type target struct {
		id, orgID, days int64
		publicID        string
	}
	var targets []target
	for rows.Next() {
		var t target
		if err := rows.Scan(&t.id, &t.publicID, &t.orgID, &t.days); err != nil {
			rows.Close()
			return 0, err
		}
		targets = append(targets, t)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	var total int64
	for _, t := range targets {
		cutoff := now.UTC().AddDate(0, 0, -int(t.days)).Format(time.RFC3339Nano)
		result, err := q.db.ExecContext(ctx, `UPDATE campaign_submissions SET triage_status = 'closed'
			WHERE campaign_id = ? AND triage_status IN ('new','reviewed') AND starred = 0 AND assignee_user_id IS NULL AND submitted_at < ?
			AND EXISTS (SELECT 1 FROM campaign_submission_reads r WHERE r.submission_id = campaign_submissions.id)`, t.id, cutoff)
		if err != nil {
			return total, err
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			continue
		}
		total += affected
		metadata, _ := json.Marshal(map[string]any{"count": affected, "days": t.days})
		if err := q.CreateAuditEvent(ctx, nil, t.orgID, "campaign_response_auto_closed", "campaign", t.publicID, nil, string(metadata)); err != nil {
			return total, err
		}
	}
	return total, nil
}

// ---- text insights ---------------------------------------------------------

type WordCount struct {
	Word  string
	Count int64
}

type DuplicateGroup struct {
	Sample string
	Count  int64
}

type TextInsights struct {
	Analyzed   int
	TopWords   []WordCount
	Duplicates []DuplicateGroup
}

func (q *Querier) ResponseTextInsights(ctx context.Context, campaignID int64) (TextInsights, error) {
	rows, err := q.db.QueryContext(ctx, `SELECT a.value_json FROM campaign_submission_answers a JOIN campaign_submissions s ON s.id = a.submission_id
		WHERE s.campaign_id = ? AND s.has_text = 1 AND a.field_type = 'textarea' ORDER BY a.id DESC LIMIT ?`, campaignID, maxInsightAnswers)
	if err != nil {
		return TextInsights{}, err
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return TextInsights{}, err
		}
		var text string
		if json.Unmarshal([]byte(raw), &text) == nil && strings.TrimSpace(text) != "" {
			texts = append(texts, text)
		}
	}
	return ComputeTextInsights(texts), rows.Err()
}

// ComputeTextInsights counts how many answers mention each word (once per answer)
// and groups identical answers.
func ComputeTextInsights(texts []string) TextInsights {
	insights := TextInsights{Analyzed: len(texts)}
	words := map[string]int64{}
	groups := map[string]*DuplicateGroup{}
	for _, text := range texts {
		seen := map[string]struct{}{}
		for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
			if len([]rune(word)) < insightMinWordRunes || insightStopwords[word] {
				continue
			}
			if _, dup := seen[word]; dup {
				continue
			}
			seen[word] = struct{}{}
			words[word]++
		}
		key := strings.Join(strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
		if len([]rune(key)) < insightMinDuplicateRune {
			continue
		}
		if group, ok := groups[key]; ok {
			group.Count++
		} else {
			sample := strings.Join(strings.Fields(text), " ")
			if runes := []rune(sample); len(runes) > 140 {
				sample = string(runes[:140]) + "..."
			}
			groups[key] = &DuplicateGroup{Sample: sample, Count: 1}
		}
	}
	for word, count := range words {
		if count >= 2 {
			insights.TopWords = append(insights.TopWords, WordCount{Word: word, Count: count})
		}
	}
	sort.Slice(insights.TopWords, func(i, j int) bool {
		if insights.TopWords[i].Count != insights.TopWords[j].Count {
			return insights.TopWords[i].Count > insights.TopWords[j].Count
		}
		return insights.TopWords[i].Word < insights.TopWords[j].Word
	})
	if len(insights.TopWords) > maxInsightWords {
		insights.TopWords = insights.TopWords[:maxInsightWords]
	}
	for _, group := range groups {
		if group.Count >= 2 {
			insights.Duplicates = append(insights.Duplicates, *group)
		}
	}
	sort.Slice(insights.Duplicates, func(i, j int) bool {
		if insights.Duplicates[i].Count != insights.Duplicates[j].Count {
			return insights.Duplicates[i].Count > insights.Duplicates[j].Count
		}
		return insights.Duplicates[i].Sample < insights.Duplicates[j].Sample
	})
	if len(insights.Duplicates) > maxInsightDuplicates {
		insights.Duplicates = insights.Duplicates[:maxInsightDuplicates]
	}
	return insights
}

var insightStopwords = func() map[string]bool {
	set := map[string]bool{}
	for _, word := range strings.Fields(`
		the and for are but not you your with this that have has had was were will would could should from they them their what when where which who whom why how
		can cant dont didnt doesnt isnt wasnt wont just very really also than then too into out off over about after before because been being its it's our ours
		any all some more most much many such only own same other another each both few get got use used using one two does doing done make made like want would
		der die das den dem des ein eine einen einem einer eines und oder aber nicht mit für von zu zum zur bei auf aus nach vor über unter ist sind war waren wird werden wurde
		wurden hat haben hatte hatten kann können konnte ich du er sie es wir ihr mein dein sein auch noch nur schon sehr mehr dass weil wenn dann doch wie was wer wo warum
		diese dieser dieses jede jeder jedes alle alles man mir mich dir dich ihn uns euch kein keine keinen
		que los las del una uno unos unas con por para como pero mas más muy sin sobre este esta esto estos estas son fue ser hay tiene tengo
		nos les mis tus sus mio mía`) {
		set[word] = true
	}
	return set
}()

// LoadSubmissionTags fills Tags for a single submission.
func (q *Querier) LoadSubmissionTags(ctx context.Context, submission *Submission) error {
	list := []Submission{*submission}
	if err := q.loadSubmissionTags(ctx, list); err != nil {
		return err
	}
	submission.Tags = list[0].Tags
	return nil
}
