package campaigns

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
)

var exportLabelPattern = regexp.MustCompile(`[^a-z0-9]+`)

func (h *Handler) Analytics(w http.ResponseWriter, r *http.Request) {
	user, campaign, role, ok := h.responseCampaign(r)
	if !ok {
		h.forbidden(w, r)
		return
	}
	rangeKey, start := analyticsRange(r.FormValue("range"), time.Now().UTC())
	analytics, err := h.q.CampaignAnalytics(r.Context(), campaign.ID, start, time.Now().UTC())
	if err != nil {
		http.Error(w, "load analytics", http.StatusInternalServerError)
		return
	}
	settings, err := h.q.GetCampaignSettings(r.Context(), campaign.ID)
	if err != nil {
		http.Error(w, "load settings", http.StatusInternalServerError)
		return
	}
	web.Render(w, r, http.StatusOK, templates.CampaignAnalyticsPage(h.cfg.InstanceName, user, campaign, settings, analytics, rangeKey, role == "owner", ""))
}

func analyticsRange(value string, now time.Time) (string, *time.Time) {
	days := 30
	key := "30"
	switch value {
	case "7":
		days, key = 7, "7"
	case "90":
		days, key = 90, "90"
	case "all":
		return "all", nil
	}
	start := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))
	return key, &start
}

func (h *Handler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.responseCampaign(r)
	if !ok {
		h.forbidden(w, r)
		return
	}
	submissions, err := h.q.ListSubmissionsWithAnswers(r.Context(), campaign.ID)
	if err != nil {
		http.Error(w, "load export", http.StatusInternalServerError)
		return
	}
	columns, labels := exportColumns(submissions)
	contextColumns := exportContextColumns(submissions)
	var output bytes.Buffer
	writer := csv.NewWriter(&output)
	header := []string{"submission_public_id", "submitted_at", "visit_public_id", "has_install_token_hash"}
	for _, key := range contextColumns {
		header = append(header, "context_"+key)
	}
	for _, publicID := range columns {
		header = append(header, "field_"+publicID+"_"+sanitizeExportLabel(labels[publicID]))
	}
	if err := writer.Write(header); err != nil {
		return
	}
	for _, submission := range submissions {
		values := map[string]string{}
		for _, answer := range submission.Answers {
			values[answer.FieldPublicID] = exportAnswerValue(answer.ValueJSON)
		}
		row := []string{submission.PublicID, submission.SubmittedAt, submission.VisitPublicID.String, strconv.FormatBool(submission.HasInstallTokenHash)}
		for _, key := range contextColumns {
			row = append(row, submission.URLContext[key])
		}
		for _, publicID := range columns {
			row = append(row, values[publicID])
		}
		if err := writer.Write(row); err != nil {
			return
		}
	}
	writer.Flush()
	if writer.Error() != nil {
		http.Error(w, "create export", http.StatusInternalServerError)
		return
	}
	if err := h.q.AuditCampaignExport(r.Context(), user.ID, campaign, "csv", len(submissions)); err != nil {
		http.Error(w, "audit export", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeExportFilename(campaign.Slug)+`-submissions.csv"`)
	_, _ = w.Write(output.Bytes())
}

type jsonExport struct {
	CampaignPublicID    string                 `json:"campaign_public_id"`
	CampaignName        string                 `json:"campaign_name"`
	ExportedAt          string                 `json:"exported_at"`
	ExportFormatVersion int                    `json:"export_format_version"`
	Submissions         []jsonExportSubmission `json:"submissions"`
}

type jsonExportSubmission struct {
	SubmissionPublicID  string             `json:"submission_public_id"`
	SubmittedAt         string             `json:"submitted_at"`
	VisitPublicID       *string            `json:"visit_public_id"`
	HasInstallTokenHash bool               `json:"has_install_token_hash"`
	URLContext          map[string]string  `json:"url_context,omitempty"`
	Answers             []jsonExportAnswer `json:"answers"`
}

type jsonExportAnswer struct {
	FieldPublicID      string `json:"field_public_id"`
	FieldType          string `json:"field_type"`
	FieldLabelSnapshot string `json:"field_label_snapshot"`
	Value              any    `json:"value"`
}

func (h *Handler) ExportJSON(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.responseCampaign(r)
	if !ok {
		h.forbidden(w, r)
		return
	}
	submissions, err := h.q.ListSubmissionsWithAnswers(r.Context(), campaign.ID)
	if err != nil {
		http.Error(w, "load export", http.StatusInternalServerError)
		return
	}
	payload := jsonExport{
		CampaignPublicID: campaign.PublicID, CampaignName: campaign.Name,
		ExportedAt: time.Now().UTC().Format(time.RFC3339Nano), ExportFormatVersion: 1,
	}
	for _, submission := range submissions {
		item := jsonExportSubmission{
			SubmissionPublicID: submission.PublicID, SubmittedAt: submission.SubmittedAt,
			HasInstallTokenHash: submission.HasInstallTokenHash, URLContext: submission.URLContext,
		}
		if submission.VisitPublicID.Valid {
			value := submission.VisitPublicID.String
			item.VisitPublicID = &value
		}
		for _, answer := range submission.Answers {
			var value any
			_ = json.Unmarshal([]byte(answer.ValueJSON), &value)
			item.Answers = append(item.Answers, jsonExportAnswer{
				FieldPublicID: answer.FieldPublicID, FieldType: answer.FieldType,
				FieldLabelSnapshot: answer.FieldLabelSnapshot, Value: value,
			})
		}
		payload.Submissions = append(payload.Submissions, item)
	}
	var output bytes.Buffer
	if err := json.NewEncoder(&output).Encode(payload); err != nil {
		http.Error(w, "create export", http.StatusInternalServerError)
		return
	}
	if err := h.q.AuditCampaignExport(r.Context(), user.ID, campaign, "json", len(submissions)); err != nil {
		http.Error(w, "audit export", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeExportFilename(campaign.Slug)+`-submissions.json"`)
	_, _ = w.Write(output.Bytes())
}

func exportContextColumns(submissions []db.Submission) []string {
	keys := map[string]struct{}{}
	for _, submission := range submissions {
		for key := range submission.URLContext {
			keys[key] = struct{}{}
		}
	}
	columns := make([]string, 0, len(keys))
	for key := range keys {
		columns = append(columns, key)
	}
	sort.Strings(columns)
	return columns
}

func exportColumns(submissions []db.Submission) ([]string, map[string]string) {
	labels := map[string]string{}
	for _, submission := range submissions {
		for _, answer := range submission.Answers {
			if _, exists := labels[answer.FieldPublicID]; !exists {
				labels[answer.FieldPublicID] = answer.FieldLabelSnapshot
			}
		}
	}
	columns := make([]string, 0, len(labels))
	for publicID := range labels {
		columns = append(columns, publicID)
	}
	sort.Strings(columns)
	return columns, labels
}

func exportAnswerValue(raw string) string {
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return ""
	}
	switch typed := value.(type) {
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			values = append(values, fmt.Sprint(item))
		}
		return strings.Join(values, ";")
	default:
		return fmt.Sprint(value)
	}
}

func sanitizeExportLabel(value string) string {
	value = exportLabelPattern.ReplaceAllString(strings.ToLower(value), "_")
	value = strings.Trim(value, "_")
	if value == "" {
		return "answer"
	}
	if len(value) > 48 {
		value = value[:48]
	}
	return value
}

func safeExportFilename(value string) string {
	value = sanitizeExportLabel(value)
	if value == "" {
		return "campaign"
	}
	return value
}

func (h *Handler) RetentionDeleteOld(w http.ResponseWriter, r *http.Request) {
	user, campaign, role, ok := h.responseCampaign(r)
	if !ok || role != "owner" {
		h.forbidden(w, r)
		return
	}
	if !requireConfirmation(w, r) {
		return
	}
	settings, err := h.q.GetCampaignSettings(r.Context(), campaign.ID)
	if err != nil || !settings.RetentionEnabled || !settings.RetentionDays.Valid {
		h.renderAnalyticsMessage(w, r, user, campaign, role, "retention.disabled")
		return
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -int(settings.RetentionDays.Int64))
	_, err = h.q.DeleteOldCampaignData(r.Context(), campaign, cutoff, user.ID)
	if err != nil {
		http.Error(w, "delete old data", http.StatusInternalServerError)
		return
	}
	web.SetFlash(w, h.cfg.Secret, h.cfg.SecureCookies, "success", "toast.retention.deleted")
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/analytics", http.StatusSeeOther)
}

func (h *Handler) DeleteAllResponses(w http.ResponseWriter, r *http.Request) {
	h.deleteAll(w, r, true)
}

func (h *Handler) DeleteAllVisits(w http.ResponseWriter, r *http.Request) {
	h.deleteAll(w, r, false)
}

func (h *Handler) deleteAll(w http.ResponseWriter, r *http.Request, responses bool) {
	user, campaign, role, ok := h.responseCampaign(r)
	if !ok || role != "owner" {
		h.forbidden(w, r)
		return
	}
	if !requireConfirmation(w, r) {
		return
	}
	if r.FormValue("confirmation") != campaign.Slug {
		h.renderAnalyticsMessage(w, r, user, campaign, role, "deletion.confirmation_error")
		return
	}
	var err error
	if responses {
		_, err = h.q.DeleteAllCampaignResponses(r.Context(), campaign, user.ID)
	} else {
		_, err = h.q.DeleteAllCampaignVisits(r.Context(), campaign, user.ID)
	}
	if err != nil {
		http.Error(w, "delete campaign data", http.StatusInternalServerError)
		return
	}
	if responses {
		web.SetFlash(w, h.cfg.Secret, h.cfg.SecureCookies, "success", "toast.responses.deleted")
	} else {
		web.SetFlash(w, h.cfg.Secret, h.cfg.SecureCookies, "success", "toast.visits.deleted")
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/analytics", http.StatusSeeOther)
}

func (h *Handler) renderAnalyticsMessage(w http.ResponseWriter, r *http.Request, user db.User, campaign db.Campaign, role, key string) {
	rangeKey, start := analyticsRange(r.FormValue("range"), time.Now().UTC())
	analytics, _ := h.q.CampaignAnalytics(r.Context(), campaign.ID, start, time.Now().UTC())
	settings, _ := h.q.GetCampaignSettings(r.Context(), campaign.ID)
	web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignAnalyticsPage(h.cfg.InstanceName, user, campaign, settings, analytics, rangeKey, role == "owner", key))
}
