package campaigns

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
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
	settings, err := h.q.GetCampaignSettings(r.Context(), campaign.ID)
	if err != nil {
		http.Error(w, "load settings", http.StatusInternalServerError)
		return
	}
	filterOptions, err := h.q.CampaignAnalyticsFilterOptions(r.Context(), campaign.ID)
	if err != nil {
		http.Error(w, "load analytics filters", http.StatusInternalServerError)
		return
	}
	rangeKey, filter := analyticsFilter(r.URL.Query(), settings, filterOptions, time.Now().UTC())
	analytics, err := h.q.CampaignAnalytics(r.Context(), campaign.ID, filter, time.Now().UTC())
	if err != nil {
		http.Error(w, "load analytics", http.StatusInternalServerError)
		return
	}
	campaigns, _ := h.q.ListCampaignsForUser(r.Context(), campaign.OrganizationID, user.ID)
	var comparison *db.CampaignAnalytics
	comparisonName := ""
	compareID := r.URL.Query().Get("compare_campaign")
	for _, candidate := range campaigns {
		if candidate.PublicID == compareID && candidate.ID != campaign.ID {
			if allowed, _ := h.permissions.CanViewCampaign(r.Context(), user.ID, candidate.ID); allowed {
				value, compareErr := h.q.CampaignAnalytics(r.Context(), candidate.ID, filter, time.Now().UTC())
				if compareErr == nil {
					comparison, comparisonName = &value, candidate.Name
				}
			}
			break
		}
	}
	web.Render(w, r, http.StatusOK, templates.CampaignAnalyticsPage(
		h.cfg.InstanceName, user, campaign, settings, analytics,
		templates.AnalyticsPageOptions{
			RangeKey: rangeKey, Filter: filter, FilterOptions: filterOptions,
			CompareCampaigns: campaigns, CompareCampaignID: compareID,
			Comparison: comparison, ComparisonName: comparisonName,
			CanDelete: role == "owner",
		},
		"",
	))
}

func analyticsRange(value string, now time.Time) (string, *time.Time, *time.Time) {
	days := 30
	key := "30"
	switch value {
	case "7":
		days, key = 7, "7"
	case "90":
		days, key = 90, "90"
	case "all":
		return "all", nil, nil
	}
	start := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -(days - 1))
	end := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1)
	return key, &start, &end
}

func analyticsFilter(values url.Values, settings db.CampaignSettings, options db.AnalyticsFilterOptions, now time.Time) (string, db.AnalyticsFilter) {
	key, start, end := analyticsRange(values.Get("range"), now)
	filter := db.AnalyticsFilter{Start: start, End: end}
	if settings.CollectURLContext {
		filter.AppVersion = allowedFilterValue(values.Get("app_version"), options.AppVersions)
		filter.ExtensionVersion = allowedFilterValue(values.Get("extension_version"), options.ExtensionVersions)
		filter.Platform = allowedFilterValue(values.Get("platform"), options.Platforms)
	}
	if settings.CollectCoarseBrowser {
		filter.Browser = allowedFilterValue(values.Get("browser"), options.Browsers)
	}
	if settings.CollectCoarseOS {
		filter.OSFamily = allowedFilterValue(values.Get("os"), options.OSFamilies)
	}
	return key, filter
}

func allowedFilterValue(value string, allowed []string) string {
	for _, candidate := range allowed {
		if value == candidate {
			return value
		}
	}
	return ""
}

func (h *Handler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.responseCampaign(r)
	if !ok {
		h.forbidden(w, r)
		return
	}
	settings, err := h.q.GetCampaignSettings(r.Context(), campaign.ID)
	if err != nil {
		http.Error(w, "load export settings", http.StatusInternalServerError)
		return
	}
	filterOptions, _ := h.q.CampaignAnalyticsFilterOptions(r.Context(), campaign.ID)
	_, filter := analyticsFilter(r.URL.Query(), settings, filterOptions, time.Now().UTC())
	submissions, err := h.q.ListSubmissionsWithAnswersFiltered(r.Context(), campaign.ID, filter)
	if err != nil {
		http.Error(w, "load export", http.StatusInternalServerError)
		return
	}
	selected := selectedExportColumns(r.URL.Query()["column"], settings)
	columns, labels := exportColumns(submissions)
	contextColumns := []string{}
	if selected["diagnostics"] && settings.CollectURLContext {
		contextColumns = exportContextColumns(submissions)
	}
	header := []string{}
	if selected["submitted_at"] {
		header = append(header, "submitted_at")
	}
	if selected["submission_id"] {
		header = append(header, "submission_id")
	}
	if selected["visit_id"] {
		header = append(header, "visit_id")
	}
	if selected["diagnostics"] {
		if settings.CollectCoarseBrowser {
			header = append(header, "browser_family")
		}
		if settings.CollectCoarseOS {
			header = append(header, "operating_system")
		}
		for _, key := range contextColumns {
			header = append(header, "context_"+key)
		}
	}
	if selected["answers"] {
		for _, publicID := range columns {
			header = append(header, "field_"+publicID+"_"+sanitizeExportLabel(labels[publicID]))
		}
	}
	if len(header) == 0 {
		http.Error(w, "select at least one export column", http.StatusUnprocessableEntity)
		return
	}
	if err := h.q.AuditCampaignExport(r.Context(), user.ID, campaign, "csv", len(submissions)); err != nil {
		http.Error(w, "audit export", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+safeExportFilename(campaign.Slug)+`-submissions.csv"`)
	_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	writer := csv.NewWriter(w)
	if err := writer.Write(header); err != nil {
		return
	}
	for _, submission := range submissions {
		values := map[string]string{}
		for _, answer := range submission.Answers {
			values[answer.FieldPublicID] = exportAnswerValue(answer.ValueJSON)
		}
		row := []string{}
		if selected["submitted_at"] {
			row = append(row, submission.SubmittedAt)
		}
		if selected["submission_id"] {
			row = append(row, submission.PublicID)
		}
		if selected["visit_id"] {
			row = append(row, submission.VisitPublicID.String)
		}
		if selected["diagnostics"] {
			if settings.CollectCoarseBrowser {
				row = append(row, submission.CoarseBrowser.String)
			}
			if settings.CollectCoarseOS {
				row = append(row, submission.CoarseOS.String)
			}
			for _, key := range contextColumns {
				row = append(row, submission.URLContext[key])
			}
		}
		if selected["answers"] {
			for _, publicID := range columns {
				row = append(row, values[publicID])
			}
		}
		for index := range row {
			row[index] = safeCSVCell(row[index])
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
}

func selectedExportColumns(values []string, settings db.CampaignSettings) map[string]bool {
	allowed := map[string]bool{"submitted_at": true, "submission_id": true, "visit_id": true, "answers": true}
	if settings.CollectURLContext || settings.CollectCoarseBrowser || settings.CollectCoarseOS {
		allowed["diagnostics"] = true
	}
	selected := map[string]bool{}
	if len(values) == 0 {
		selected = map[string]bool{"submitted_at": true, "submission_id": true, "visit_id": true, "answers": true}
		if allowed["diagnostics"] {
			selected["diagnostics"] = true
		}
		return selected
	}
	for _, value := range values {
		if allowed[value] {
			selected[value] = true
		}
	}
	return selected
}

func safeCSVCell(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if trimmed != "" && strings.ContainsRune("=+-@", rune(trimmed[0])) {
		return "'" + value
	}
	return value
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
	settings, err := h.q.GetCampaignSettings(r.Context(), campaign.ID)
	if err != nil {
		http.Error(w, "load export settings", http.StatusInternalServerError)
		return
	}
	filterOptions, _ := h.q.CampaignAnalyticsFilterOptions(r.Context(), campaign.ID)
	_, filter := analyticsFilter(r.URL.Query(), settings, filterOptions, time.Now().UTC())
	submissions, err := h.q.ListSubmissionsWithAnswersFiltered(r.Context(), campaign.ID, filter)
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
			HasInstallTokenHash: submission.HasInstallTokenHash,
		}
		if settings.CollectURLContext {
			item.URLContext = submission.URLContext
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
	settings, _ := h.q.GetCampaignSettings(r.Context(), campaign.ID)
	filterOptions, _ := h.q.CampaignAnalyticsFilterOptions(r.Context(), campaign.ID)
	rangeKey, filter := analyticsFilter(r.URL.Query(), settings, filterOptions, time.Now().UTC())
	analytics, _ := h.q.CampaignAnalytics(r.Context(), campaign.ID, filter, time.Now().UTC())
	web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignAnalyticsPage(h.cfg.InstanceName, user, campaign, settings, analytics, templates.AnalyticsPageOptions{
		RangeKey: rangeKey, Filter: filter, FilterOptions: filterOptions, CanDelete: role == "owner",
	}, key))
}
