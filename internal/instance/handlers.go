package instance

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/i18n"
	"github.com/koalastuff/koalabye/internal/permissions"
	"github.com/koalastuff/koalabye/internal/version"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
)

type Handler struct {
	cfg         config.Config
	queries     *db.Querier
	permissions *permissions.Service
}

func (h *Handler) Users(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	users, err := h.queries.ListInstanceUsers(r.Context())
	if err != nil {
		http.Error(w, "load users", 500)
		return
	}
	web.Render(w, r, 200, templates.InstanceUsers(h.cfg.InstanceName, u, users))
}
func (h *Handler) UserStatus(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	disabled := r.FormValue("disabled") == "true"
	if err := h.queries.SetUserDisabled(r.Context(), r.FormValue("public_id"), disabled, u.ID); err != nil {
		http.Error(w, "action denied", 422)
		return
	}
	http.Redirect(w, r, "/instance/users", 303)
}
func (h *Handler) Organizations(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	orgs, err := h.queries.ListInstanceOrganizations(r.Context())
	if err != nil {
		http.Error(w, "load organizations", 500)
		return
	}
	web.Render(w, r, 200, templates.InstanceOrganizations(h.cfg.InstanceName, u, orgs))
}
func (h *Handler) OrganizationStatus(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	disabled := r.FormValue("disabled") == "true"
	if err := h.queries.SetOrganizationDisabled(r.Context(), r.FormValue("public_id"), disabled, u.ID); err != nil {
		http.Error(w, "action failed", 422)
		return
	}
	http.Redirect(w, r, "/instance/organizations", 303)
}
func (h *Handler) Campaigns(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	campaigns, err := h.queries.ListInstanceCampaigns(r.Context())
	if err != nil {
		http.Error(w, "load campaigns", http.StatusInternalServerError)
		return
	}
	web.Render(w, r, http.StatusOK, templates.InstanceCampaigns(h.cfg.InstanceName, u, campaigns))
}
func (h *Handler) CampaignStatus(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	if err := h.queries.SetCampaignDisabled(r.Context(), r.FormValue("public_id"), r.FormValue("disabled") == "true", u.ID); err != nil {
		http.Error(w, "action failed", http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/instance/campaigns", http.StatusSeeOther)
}
func (h *Handler) Limits(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	org, err := h.queries.GetOrganizationByPublicID(r.Context(), r.URL.Query().Get("org"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	limits, _ := h.queries.GetOrganizationLimits(r.Context(), org.ID)
	web.Render(w, r, 200, templates.InstanceLimits(h.cfg.InstanceName, u, org, limits))
}
func (h *Handler) LimitsPost(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	org, err := h.queries.GetOrganizationByPublicID(r.Context(), r.FormValue("public_id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	limits, err := parseOrganizationLimits(r)
	if err != nil {
		http.Error(w, "invalid limits", 422)
		return
	}
	if err = h.queries.UpdateOrganizationLimits(r.Context(), org.ID, limits, u.ID); err != nil {
		http.Error(w, "update failed", 422)
		return
	}
	http.Redirect(w, r, "/instance/organizations", 303)
}
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	settings, _ := h.queries.Settings(r.Context())
	web.Render(w, r, 200, templates.InstanceSettings(h.cfg.InstanceName, u, settings))
}
func (h *Handler) SettingsPost(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	values := map[string]string{
		"registration_enabled":                  boolString(r.FormValue("registration_enabled") == "on"),
		"invite_only":                           boolString(r.FormValue("invite_only") == "on"),
		"invite_registration_enabled":           boolString(r.FormValue("invite_registration_enabled") == "on"),
		"instance_name":                         strings.TrimSpace(r.FormValue("instance_name")),
		"instance_operator_name":                strings.TrimSpace(r.FormValue("instance_operator_name")),
		"instance_legal_pages_are_placeholders": boolString(r.FormValue("instance_legal_pages_are_placeholders") == "on"),
	}
	for _, key := range []string{"instance_operator_url", "instance_legal_notice_url", "instance_privacy_policy_url", "instance_source_url", "instance_contact_url", "instance_support_url"} {
		value, valid := safeURL(r.FormValue(key))
		if !valid {
			http.Error(w, "invalid URL", http.StatusUnprocessableEntity)
			return
		}
		values[key] = value
	}
	for _, k := range []string{"default_max_organizations_per_user", "default_max_campaigns_per_org", "default_max_members_per_org", "default_max_active_invites_per_org", "default_max_monthly_visits_per_org", "default_max_monthly_submissions_per_org"} {
		v, err := strconv.Atoi(r.FormValue(k))
		minimum := 0
		if k == "default_max_organizations_per_user" || k == "default_max_members_per_org" {
			minimum = 1
		}
		if err != nil || v < minimum {
			http.Error(w, "invalid setting", 422)
			return
		}
		values[k] = strconv.Itoa(v)
	}
	if strings.TrimSpace(values["instance_name"]) == "" {
		http.Error(w, "invalid instance name", http.StatusUnprocessableEntity)
		return
	}
	if err := h.queries.UpdateSettings(r.Context(), values, u.ID); err != nil {
		http.Error(w, "update failed", 500)
		return
	}
	http.Redirect(w, r, "/instance/settings", 303)
}
func (h *Handler) Audit(w http.ResponseWriter, r *http.Request) {
	u, ok := h.authorize(w, r)
	if !ok {
		return
	}
	events, _ := h.queries.ListRecentAuditEvents(r.Context(), 100)
	web.Render(w, r, 200, templates.InstanceAudit(h.cfg.InstanceName, u, events))
}
func (h *Handler) authorize(w http.ResponseWriter, r *http.Request) (db.User, bool) {
	u, _ := auth.UserFromContext(r.Context())
	allowed, err := h.permissions.CanAccessInstanceAdmin(r.Context(), u.ID)
	if err != nil || !allowed {
		web.Render(w, r, 403, templates.ErrorPage(h.cfg.InstanceName, 403, i18n.T(r.Context(), "error.forbidden.title"), i18n.T(r.Context(), "error.forbidden.message")))
		return u, false
	}
	return u, true
}
func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func safeURL(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", true
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	if parsed.Scheme == "https" {
		return raw, true
	}
	if parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1") {
		return raw, true
	}
	return "", false
}

func parseOrganizationLimits(r *http.Request) (db.OrganizationLimits, error) {
	parse := func(key string, minimum, maximum int64) (int64, error) {
		value, err := strconv.ParseInt(strings.TrimSpace(r.FormValue(key)), 10, 64)
		if err != nil || value < minimum || value > maximum {
			return 0, errors.New("invalid limit")
		}
		return value, nil
	}
	campaigns, err := parse("max_campaigns", 0, 10000)
	if err != nil {
		return db.OrganizationLimits{}, err
	}
	members, err := parse("max_members", 1, 10000)
	if err != nil {
		return db.OrganizationLimits{}, err
	}
	invites, err := parse("max_active_invites", 0, 10000)
	if err != nil {
		return db.OrganizationLimits{}, err
	}
	visits, err := parse("max_monthly_visits", 0, 1000000000)
	if err != nil {
		return db.OrganizationLimits{}, err
	}
	submissions, err := parse("max_monthly_submissions", 0, 1000000000)
	if err != nil {
		return db.OrganizationLimits{}, err
	}
	return db.OrganizationLimits{
		MaxCampaigns:          campaigns,
		MaxMembers:            members,
		MaxActiveInvites:      invites,
		MaxMonthlyVisits:      visits,
		MaxMonthlySubmissions: submissions,
	}, nil
}

func New(cfg config.Config, queries *db.Querier, permissionService *permissions.Service) *Handler {
	return &Handler{cfg: cfg, queries: queries, permissions: permissionService}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	allowed, err := h.permissions.CanAccessInstanceAdmin(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "could not check permissions", http.StatusInternalServerError)
		return
	}
	if !allowed {
		web.Render(w, r, http.StatusForbidden, templates.ErrorPage(
			h.cfg.InstanceName,
			http.StatusForbidden,
			i18n.T(r.Context(), "error.forbidden.title"),
			i18n.T(r.Context(), "error.forbidden.message"),
		))
		return
	}
	registration, err := h.queries.GetSetting(r.Context(), "registration_enabled")
	if err != nil {
		registration = "unknown"
	}
	events, err := h.queries.ListRecentAuditEvents(r.Context(), 20)
	if err != nil {
		http.Error(w, "could not load audit log", http.StatusInternalServerError)
		return
	}
	build := version.Current()
	web.Render(w, r, http.StatusOK, templates.Instance(h.cfg.InstanceName+" "+build.Version, user, registration, events))
}
