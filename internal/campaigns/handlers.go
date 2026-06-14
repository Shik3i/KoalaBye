package campaigns

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/i18n"
	"github.com/koalastuff/koalabye/internal/ids"
	"github.com/koalastuff/koalabye/internal/permissions"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var contextValuePattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

var allowedContextKeys = []string{
	"app_version",
	"extension_version",
	"platform",
	"source",
	"channel",
	"utm_source",
	"utm_medium",
	"utm_campaign",
	"utm_content",
	"utm_term",
}

type Handler struct {
	cfg         config.Config
	q           *db.Querier
	permissions *permissions.Service
}

func New(cfg config.Config, q *db.Querier, p *permissions.Service) *Handler {
	return &Handler{cfg: cfg, q: q, permissions: p}
}

func (h *Handler) logError(ctx context.Context, msg string, err error) {
	if err != nil {
		h.q.CreateErrorLog(ctx, "error", msg+": "+err.Error(), map[string]string{"error": err.Error()})
	}
}

func (h *Handler) PublicByID(w http.ResponseWriter, r *http.Request) {
	h.public(w, r, func() (db.PublicCampaign, error) {
		return h.q.GetPublicCampaignByID(r.Context(), chi.URLParam(r, "campaignPublicID"))
	})
}

func (h *Handler) PublicBySlug(w http.ResponseWriter, r *http.Request) {
	h.public(w, r, func() (db.PublicCampaign, error) {
		return h.q.GetPublicCampaignBySlug(r.Context(), chi.URLParam(r, "orgSlug"), chi.URLParam(r, "campaignSlug"))
	})
}

func (h *Handler) public(w http.ResponseWriter, r *http.Request, resolve func() (db.PublicCampaign, error)) {
	publicCampaign, err := resolve()
	if err != nil {
		h.publicUnavailable(w, r, http.StatusNotFound, false, "en")
		return
	}
	if !publicCampaign.Available() {
		h.publicUnavailable(w, r, http.StatusNotFound, false, publicCampaign.Settings.PublicLanguageDefault)
		return
	}
	r = r.WithContext(i18n.PublicCampaignContext(r.Context(), r, publicCampaign.Settings.PublicLanguageDefault))
	input := h.visitInput(r, publicCampaign)
	publicID, err := ids.New("visit")
	if err != nil {
		h.publicUnavailable(w, r, http.StatusServiceUnavailable, false, publicCampaign.Settings.PublicLanguageDefault)
		return
	}
	input.PublicID = publicID
	recordedPublicID, err := h.q.RecordCampaignVisitWithPublicID(r.Context(), input)
	if errors.Is(err, db.ErrVisitLimitReached) {
		h.publicUnavailable(w, r, http.StatusServiceUnavailable, true, publicCampaign.Settings.PublicLanguageDefault)
		return
	} else if err != nil {
		h.publicUnavailable(w, r, http.StatusServiceUnavailable, false, publicCampaign.Settings.PublicLanguageDefault)
		return
	}
	if recordedPublicID != "" {
		publicID = recordedPublicID
	}
	fields, err := h.q.ListFormFields(r.Context(), publicCampaign.Campaign.ID, false)
	if err != nil {
		h.publicUnavailable(w, r, http.StatusServiceUnavailable, false, publicCampaign.Settings.PublicLanguageDefault)
		return
	}
	web.Render(w, r, http.StatusOK, templates.PublicCampaignPage(h.cfg.InstanceName, publicCampaign.Campaign, publicCampaign.Settings, publicCampaign.Branding, fields, publicID, ""))
}

func (h *Handler) publicUnavailable(w http.ResponseWriter, r *http.Request, status int, quota bool, defaultLocale string) {
	r = r.WithContext(i18n.PublicCampaignContext(r.Context(), r, defaultLocale))
	web.Render(w, r, status, templates.PublicCampaignUnavailable(h.cfg.InstanceName, quota))
}

func (h *Handler) visitInput(r *http.Request, publicCampaign db.PublicCampaign) db.RecordVisitInput {
	settings := publicCampaign.Settings
	tokenHash := ""
	token := r.URL.Query().Get("t")
	if settings.CollectInstallToken && token != "" && len(token) <= 256 {
		tokenHash = hashInstallToken(h.cfg.Secret, token)
	}
	referrer := ""
	if settings.CollectReferrerDomain {
		referrer = referrerDomain(r.Header.Get("Referer"))
	}
	browser, os := "", ""
	if settings.CollectCoarseBrowser || settings.CollectCoarseOS {
		browser, os = coarseUserAgent(r.Header.Get("User-Agent"))
		if !settings.CollectCoarseBrowser {
			browser = ""
		}
		if !settings.CollectCoarseOS {
			os = ""
		}
	}
	return db.RecordVisitInput{
		CampaignID: publicCampaign.Campaign.ID, OrganizationID: publicCampaign.Campaign.OrganizationID,
		TokenHash: tokenHash, ReferrerDomain: referrer, CoarseBrowser: browser, CoarseOS: os,
		URLContext: safeURLContext(r.URL.Query(), settings.CollectURLContext),
		CountRaw:   settings.CountRawVisits, CountUnique: settings.CountUniqueTokenVisits,
		CollectToken: settings.CollectInstallToken, CreatedAt: time.Now().UTC(),
	}
}

func safeURLContext(values url.Values, enabled bool) map[string]string {
	if !enabled {
		return nil
	}
	contextValues := make(map[string]string)
	for _, key := range allowedContextKeys {
		value := strings.TrimSpace(values.Get(key))
		lower := strings.ToLower(value)
		if !contextValuePattern.MatchString(value) ||
			strings.Contains(lower, "javascript:") ||
			strings.Contains(lower, "data:") ||
			strings.Contains(lower, "vbscript:") ||
			strings.Contains(lower, "://") {
			continue
		}
		contextValues[key] = value
	}
	return contextValues
}

func hashInstallToken(secret, token string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(token))
	return hex.EncodeToString(mac.Sum(nil))
}

func referrerDomain(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		return ""
	}
	return strings.ToLower(parsed.Hostname())
}

func coarseUserAgent(raw string) (string, string) {
	value := strings.ToLower(raw)
	browser := "Unknown"
	switch {
	case value == "":
	case strings.Contains(value, "edg/"):
		browser = "Edge"
	case strings.Contains(value, "firefox/") || strings.Contains(value, "fxios/"):
		browser = "Firefox"
	case strings.Contains(value, "chrome/") || strings.Contains(value, "crios/"):
		browser = "Chrome"
	case strings.Contains(value, "safari/"):
		browser = "Safari"
	default:
		browser = "Other"
	}
	os := "Unknown"
	switch {
	case value == "":
	case strings.Contains(value, "android"):
		os = "Android"
	case strings.Contains(value, "iphone") || strings.Contains(value, "ipad") || strings.Contains(value, "ios"):
		os = "iOS"
	case strings.Contains(value, "windows"):
		os = "Windows"
	case strings.Contains(value, "mac os") || strings.Contains(value, "macintosh"):
		os = "macOS"
	case strings.Contains(value, "linux"):
		os = "Linux"
	default:
		os = "Other"
	}
	return browser, os
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	org, role, instanceOwner, ok := h.organization(r, user.ID)
	if !ok {
		h.forbidden(w, r)
		return
	}
	items, err := h.q.ListCampaignsForUser(r.Context(), org.ID, user.ID)
	if instanceOwner {
		items, err = h.q.ListCampaignsForOrg(r.Context(), org.ID)
	}
	if err != nil {
		h.logError(r.Context(), "load campaigns", err)
		http.Error(w, "load campaigns", http.StatusInternalServerError)
		return
	}
	limits, _ := h.q.GetOrganizationLimits(r.Context(), org.ID)
	count, _ := h.q.CountCampaigns(r.Context(), org.ID)
	canCreate := (role == "owner" || role == "admin") && count < limits.MaxCampaigns
	web.Render(w, r, http.StatusOK, templates.CampaignList(h.cfg.InstanceName, user, org, items, limits, count, canCreate))
}

func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	org, role, _, ok := h.organization(r, user.ID)
	if !ok || (role != "owner" && role != "admin") {
		h.forbidden(w, r)
		return
	}
	web.Render(w, r, http.StatusOK, templates.CampaignNew(h.cfg.InstanceName, user, org, "", db.PrivacyPreset("strict")))
}

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	org, role, _, ok := h.organization(r, user.ID)
	if !ok || (role != "owner" && role != "admin") {
		h.forbidden(w, r)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(strings.ToLower(r.FormValue("slug")))
	description := strings.TrimSpace(r.FormValue("description"))
	language := validLanguage(r.FormValue("public_language_default"))
	preset := r.FormValue("privacy_preset")
	if name == "" || len(name) > 120 || len(slug) < 2 || len(slug) > 80 || !slugPattern.MatchString(slug) || (preset != "strict" && preset != "balanced") {
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignNew(h.cfg.InstanceName, user, org, "campaign.error.invalid", db.PrivacyPreset("strict")))
		return
	}
	publicID, err := ids.New("camp")
	if err != nil {
		h.logError(r.Context(), "create campaign", err)
		http.Error(w, "create campaign", http.StatusInternalServerError)
		return
	}
	campaign, err := h.q.CreateCampaign(r.Context(), db.CreateCampaignInput{
		PublicID: publicID, OrganizationID: org.ID, CreatedBy: user.ID, Name: name,
		Slug: slug, Description: description, Language: language, PrivacyPreset: preset,
	})
	if errors.Is(err, db.ErrLimitReached) {
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignNew(h.cfg.InstanceName, user, org, "campaign.error.limit", db.PrivacyPreset(preset)))
		return
	}
	if err != nil {
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignNew(h.cfg.InstanceName, user, org, "campaign.error.slug", db.PrivacyPreset(preset)))
		return
	}
	formPreset := r.FormValue("form_preset")
	if formPreset != "" && formPreset != "none" {
		if err := ApplyFormPreset(r.Context(), h.q, campaign.ID, formPreset, language, user.ID); err != nil {
			slog.ErrorContext(r.Context(), "apply campaign form preset", "campaign_public_id", campaign.PublicID, "preset", formPreset, "error", err)
		}
	}
	http.Redirect(w, r, campaignURL(org.PublicID, campaign.PublicID), http.StatusSeeOther)
}

func (h *Handler) Detail(w http.ResponseWriter, r *http.Request) {
	user, campaign, role, ok := h.campaign(r, permissionView)
	if !ok {
		h.forbidden(w, r)
		return
	}
	settings, err := h.q.GetCampaignSettings(r.Context(), campaign.ID)
	if err != nil {
		h.logError(r.Context(), "load settings", err)
		http.Error(w, "load settings", http.StatusInternalServerError)
		return
	}
	stats, err := h.q.CampaignVisitStats(r.Context(), campaign.ID, time.Now())
	if err != nil {
		h.logError(r.Context(), "load campaign visits", err)
		http.Error(w, "load campaign visits", http.StatusInternalServerError)
		return
	}
	submissionStats, err := h.q.SubmissionStats(r.Context(), campaign.ID, time.Now())
	if err != nil {
		h.logError(r.Context(), "load campaign submissions", err)
		http.Error(w, "load campaign submissions", http.StatusInternalServerError)
		return
	}
	fields, err := h.q.ListFormFields(r.Context(), campaign.ID, false)
	if err != nil {
		h.logError(r.Context(), "load campaign form", err)
		http.Error(w, "load campaign form", http.StatusInternalServerError)
		return
	}
	web.Render(w, r, http.StatusOK, templates.CampaignDetail(h.cfg.InstanceName, user, campaign, settings, stats, submissionStats, len(fields), role, strings.TrimRight(h.cfg.BaseURL, "/")))
}

func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	web.Render(w, r, http.StatusOK, templates.CampaignSettings(h.cfg.InstanceName, user, campaign, ""))
}

func (h *Handler) Branding(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	branding, _ := h.q.GetCampaignBranding(r.Context(), campaign.ID)
	web.Render(w, r, http.StatusOK, templates.CampaignBrandingForm(h.cfg.InstanceName, user, campaign, branding, ""))
}

func (h *Handler) BrandingPost(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	branding := db.CampaignBranding{
		BrandName:            nullableString(strings.TrimSpace(r.FormValue("brand_name"))),
		BrandURL:             nullableString(safeURL(r.FormValue("brand_url"))),
		PrivacyPolicyURL:     nullableString(safeURL(r.FormValue("privacy_policy_url"))),
		LegalNoticeURL:       nullableString(safeURL(r.FormValue("legal_notice_url"))),
		SupportURL:           nullableString(safeURL(r.FormValue("support_url"))),
		ContactURL:           nullableString(safeURL(r.FormValue("contact_url"))),
		AccentPreset:         strings.TrimSpace(r.FormValue("accent_preset")),
		BackgroundStyle:      strings.TrimSpace(r.FormValue("background_style")),
		ShowKoalabyeBranding: r.FormValue("show_koalabye_branding") == "on",
		PublicHeading:        nullableString(strings.TrimSpace(r.FormValue("public_heading"))),
		PublicIntro:          nullableString(strings.TrimSpace(r.FormValue("public_intro"))),
	}
	if err := h.q.UpdateCampaignBranding(r.Context(), campaign, branding, user.ID); err != nil {
		h.logError(r.Context(), "branding update failed", err)
		http.Error(w, "branding update failed", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/branding", http.StatusSeeOther)
}

func (h *Handler) SettingsPost(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionEdit)
	if !ok {
		h.forbidden(w, r)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	slug := strings.TrimSpace(strings.ToLower(r.FormValue("slug")))
	if name == "" || len(name) > 120 || len(slug) < 2 || len(slug) > 80 || !slugPattern.MatchString(slug) {
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignSettings(h.cfg.InstanceName, user, campaign, "campaign.error.invalid"))
		return
	}
	campaign.Name, campaign.Slug = name, slug
	campaign.Description = nullableString(strings.TrimSpace(r.FormValue("description")))
	campaign.PublicLinkEnabled = r.FormValue("public_link_enabled") == "on"
	if err := h.q.UpdateCampaign(r.Context(), campaign, user.ID); err != nil {
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignSettings(h.cfg.InstanceName, user, campaign, "campaign.error.slug"))
		return
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID), http.StatusSeeOther)
}

func (h *Handler) Privacy(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionPrivacy)
	if !ok {
		h.forbidden(w, r)
		return
	}
	settings, _ := h.q.GetCampaignSettings(r.Context(), campaign.ID)
	web.Render(w, r, http.StatusOK, templates.CampaignPrivacy(h.cfg.InstanceName, user, campaign, settings, ""))
}

func (h *Handler) PrivacyPost(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionPrivacy)
	if !ok {
		h.forbidden(w, r)
		return
	}
	settings := db.CampaignSettings{
		CollectInstallToken: r.FormValue("collect_install_token") == "on", HashInstallToken: true,
		CountRawVisits: r.FormValue("count_raw_visits") == "on", CountUniqueTokenVisits: r.FormValue("count_unique_token_visits") == "on",
		CollectReferrerDomain: r.FormValue("collect_referrer_domain") == "on", CollectCoarseBrowser: r.FormValue("collect_coarse_browser") == "on",
		CollectCoarseOS: r.FormValue("collect_coarse_os") == "on", CollectURLContext: r.FormValue("collect_url_context") == "on",
		PublicLanguageDefault: validLanguage(r.FormValue("public_language_default")), ShowPrivacyNotice: true,
		RetentionEnabled: r.FormValue("retention_enabled") == "on",
	}
	if settings.RetentionEnabled {
		days, err := strconv.ParseInt(r.FormValue("retention_days"), 10, 64)
		if err != nil || (days != 30 && days != 90 && days != 180 && days != 365) {
			current, _ := h.q.GetCampaignSettings(r.Context(), campaign.ID)
			web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignPrivacy(h.cfg.InstanceName, user, campaign, current, "retention.invalid"))
			return
		}
		settings.RetentionDays = sql.NullInt64{Int64: days, Valid: true}
	}
	if err := h.q.UpdateCampaignPrivacy(r.Context(), campaign, settings, user.ID); err != nil {
		http.Error(w, "privacy update denied", http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/privacy", http.StatusSeeOther)
}

func (h *Handler) Access(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionAccess)
	if !ok {
		h.forbidden(w, r)
		return
	}
	members, _ := h.q.ListCampaignAccess(r.Context(), campaign.ID, campaign.OrganizationID)
	web.Render(w, r, http.StatusOK, templates.CampaignAccess(h.cfg.InstanceName, user, campaign, members, ""))
}

func (h *Handler) AccessPost(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionAccess)
	if !ok {
		h.forbidden(w, r)
		return
	}
	err := h.q.SetCampaignMember(r.Context(), campaign, r.FormValue("user_public_id"), r.FormValue("role"), user.ID)
	if err != nil {
		members, _ := h.q.ListCampaignAccess(r.Context(), campaign.ID, campaign.OrganizationID)
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignAccess(h.cfg.InstanceName, user, campaign, members, campaignAccessError(err)))
		return
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/access", http.StatusSeeOther)
}

func (h *Handler) AccessRemove(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionAccess)
	if !ok {
		h.forbidden(w, r)
		return
	}
	err := h.q.RemoveCampaignMember(r.Context(), campaign, chi.URLParam(r, "userPublicID"), user.ID)
	if err != nil {
		members, _ := h.q.ListCampaignAccess(r.Context(), campaign.ID, campaign.OrganizationID)
		web.Render(w, r, http.StatusUnprocessableEntity, templates.CampaignAccess(h.cfg.InstanceName, user, campaign, members, campaignAccessError(err)))
		return
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID)+"/access", http.StatusSeeOther)
}

func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	user, campaign, _, ok := h.campaign(r, permissionArchive)
	if !ok {
		h.forbidden(w, r)
		return
	}
	if err := h.q.ChangeCampaignStatus(r.Context(), campaign, r.FormValue("status"), user.ID); err != nil {
		http.Error(w, "invalid status transition", http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, campaignURL(campaign.OrganizationPublicID, campaign.PublicID), http.StatusSeeOther)
}

type permissionKind int

const (
	permissionView permissionKind = iota
	permissionEdit
	permissionPrivacy
	permissionAccess
	permissionArchive
)

func (h *Handler) campaign(r *http.Request, permission permissionKind) (db.User, db.Campaign, string, bool) {
	user, _ := auth.UserFromContext(r.Context())
	campaign, err := h.q.GetCampaignByPublicID(r.Context(), chi.URLParam(r, "orgPublicID"), chi.URLParam(r, "campaignPublicID"))
	if err != nil {
		return user, campaign, "", false
	}
	role, err := h.permissions.CampaignRole(r.Context(), user.ID, campaign.ID)
	if err != nil {
		return user, campaign, "", false
	}
	allowed := false
	switch permission {
	case permissionView:
		allowed, _ = h.permissions.CanViewCampaign(r.Context(), user.ID, campaign.ID)
	case permissionEdit:
		allowed, _ = h.permissions.CanEditCampaign(r.Context(), user.ID, campaign.ID)
	case permissionPrivacy:
		allowed, _ = h.permissions.CanChangeCampaignPrivacy(r.Context(), user.ID, campaign.ID)
	case permissionAccess:
		allowed, _ = h.permissions.CanManageCampaignAccess(r.Context(), user.ID, campaign.ID)
	case permissionArchive:
		allowed, _ = h.permissions.CanArchiveCampaign(r.Context(), user.ID, campaign.ID)
	}
	if permission != permissionView && (campaign.Status == "archived" || campaign.DisabledAt.Valid) {
		allowed = false
	}
	return user, campaign, role, allowed
}

func (h *Handler) organization(r *http.Request, userID int64) (db.Organization, string, bool, bool) {
	org, err := h.q.GetOrganizationByPublicID(r.Context(), chi.URLParam(r, "orgPublicID"))
	if err != nil || org.DisabledAt.Valid {
		return org, "", false, false
	}
	instanceOwner, _ := h.permissions.IsInstanceOwner(r.Context(), userID)
	role, err := h.q.OrganizationRole(r.Context(), userID, org.ID)
	if instanceOwner {
		return org, role, true, true
	}
	return org, role, false, err == nil
}

func (h *Handler) forbidden(w http.ResponseWriter, r *http.Request) {
	web.Render(w, r, http.StatusForbidden, templates.ErrorPage(h.cfg.InstanceName, http.StatusForbidden, i18n.T(r.Context(), "error.forbidden.title"), i18n.T(r.Context(), "error.forbidden.message")))
}

func campaignURL(orgPublicID, campaignPublicID string) string {
	return "/app/orgs/" + orgPublicID + "/campaigns/" + campaignPublicID
}

func validLanguage(value string) string {
	if value == "de" || value == "es" {
		return value
	}
	return "en"
}

func nullableString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func safeURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "https" {
		return raw
	}
	if parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1") {
		return raw
	}
	return ""
}

func campaignAccessError(err error) string {
	if errors.Is(err, db.ErrLastOwner) {
		return "campaign.access.error.last_owner"
	}
	if errors.Is(err, db.ErrCampaignArchived) {
		return "campaign.error.archived"
	}
	return "campaign.access.error.member"
}
