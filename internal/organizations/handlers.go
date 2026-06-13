package organizations

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/ids"
	"github.com/koalastuff/koalabye/internal/permissions"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
)

var slugCleaner = regexp.MustCompile(`[^a-z0-9]+`)

type Handler struct {
	cfg         config.Config
	q           *db.Querier
	csrf        *auth.CSRF
	permissions *permissions.Service
}

func New(cfg config.Config, q *db.Querier, csrf *auth.CSRF, p *permissions.Service) *Handler {
	return &Handler{cfg: cfg, q: q, csrf: csrf, permissions: p}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	orgs, err := h.q.ListOrganizationsForUser(r.Context(), u.ID)
	if err != nil {
		http.Error(w, "load organizations", 500)
		return
	}
	settings, _ := h.q.Settings(r.Context())
	limit, _ := strconv.Atoi(settings["default_max_organizations_per_user"])
	count, _ := h.q.CountOrganizationsCreatedByUser(r.Context(), u.ID)
	web.Render(w, r, 200, templates.Organizations(h.cfg.InstanceName, u, orgs, count < limit))
}
func (h *Handler) New(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	web.Render(w, r, 200, templates.OrganizationNew(h.cfg.InstanceName, u, ""))
}
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	if r.ParseForm() != nil || h.csrf.Validate(r) != nil {
		http.Error(w, "csrf", 403)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	slug := slugify(r.FormValue("slug"))
	if name == "" || slug == "" {
		web.Render(w, r, 422, templates.OrganizationNew(h.cfg.InstanceName, u, "org.error.invalid"))
		return
	}
	publicID, _ := ids.New("org")
	org, err := h.q.CreateOrganization(r.Context(), db.CreateOrganizationInput{PublicID: publicID, Slug: slug, Name: name, UserID: u.ID, Limits: h.defaultLimits(r)})
	if errors.Is(err, db.ErrLimitReached) {
		web.Render(w, r, 422, templates.OrganizationNew(h.cfg.InstanceName, u, "org.error.limit"))
		return
	}
	if err != nil {
		web.Render(w, r, 422, templates.OrganizationNew(h.cfg.InstanceName, u, "org.error.create"))
		return
	}
	http.Redirect(w, r, "/app/orgs/"+org.PublicID, 303)
}

func (h *Handler) View(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	org, role, owner, ok := h.authorize(r, u.ID, false)
	if !ok {
		h.forbidden(w, r)
		return
	}
	members, _ := h.q.ListOrganizationMembers(r.Context(), org.ID)
	limits, _ := h.q.GetOrganizationLimits(r.Context(), org.ID)
	campaigns, _ := h.q.ListCampaignsForUser(r.Context(), org.ID, u.ID)
	canManage := owner || role == "owner" || role == "admin"
	web.Render(w, r, 200, templates.OrganizationDetail(h.cfg.InstanceName, u, org, role, members, limits, campaigns, canManage))
}
func (h *Handler) Settings(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	org, role, owner, ok := h.authorize(r, u.ID, true)
	if !ok {
		h.forbidden(w, r)
		return
	}
	limits, _ := h.q.GetOrganizationLimits(r.Context(), org.ID)
	web.Render(w, r, 200, templates.OrganizationSettings(h.cfg.InstanceName, u, org, role, limits, owner))
}
func (h *Handler) SettingsPost(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	org, _, _, ok := h.authorize(r, u.ID, true)
	if !ok {
		h.forbidden(w, r)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	slug := slugify(r.FormValue("slug"))
	if name == "" || slug == "" {
		http.Error(w, "invalid organization", http.StatusUnprocessableEntity)
		return
	}
	if err := h.q.UpdateOrganization(r.Context(), org.ID, u.ID, name, slug); err != nil {
		http.Error(w, "update failed", http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/app/orgs/"+org.PublicID+"/settings", http.StatusSeeOther)
}
func (h *Handler) Members(w http.ResponseWriter, r *http.Request) { h.View(w, r) }
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	org, role, owner, ok := h.authorize(r, u.ID, true)
	if !ok {
		h.forbidden(w, r)
		return
	}
	if r.ParseForm() != nil || h.csrf.Validate(r) != nil {
		http.Error(w, "csrf", 403)
		return
	}
	target, _ := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
	err := h.q.RemoveMember(r.Context(), org.ID, target, u.ID, role, owner)
	if err != nil {
		http.Error(w, "member removal denied", 422)
		return
	}
	http.Redirect(w, r, "/app/orgs/"+org.PublicID, 303)
}
func (h *Handler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	org, role, owner, ok := h.authorize(r, u.ID, true)
	if !ok {
		h.forbidden(w, r)
		return
	}
	target, _ := strconv.ParseInt(r.FormValue("user_id"), 10, 64)
	if err := h.q.UpdateMemberRole(r.Context(), org.ID, target, u.ID, role, r.FormValue("role"), owner); err != nil {
		http.Error(w, "role update denied", http.StatusUnprocessableEntity)
		return
	}
	http.Redirect(w, r, "/app/orgs/"+org.PublicID, http.StatusSeeOther)
}
func (h *Handler) Invites(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	org, role, instanceOwner, ok := h.authorize(r, u.ID, true)
	if !ok || !(role == "owner" || role == "admin" || instanceOwner) {
		h.forbidden(w, r)
		return
	}
	invites, _ := h.q.ListInvites(r.Context(), org.ID)
	limits, _ := h.q.GetOrganizationLimits(r.Context(), org.ID)
	web.Render(w, r, 200, templates.OrganizationInvites(h.cfg.InstanceName, u, org, invites, limits, "", ""))
}
func (h *Handler) CreateInvite(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	org, role, instanceOwner, ok := h.authorize(r, u.ID, true)
	if !ok || !(role == "owner" || role == "admin" || instanceOwner) {
		h.forbidden(w, r)
		return
	}
	if r.ParseForm() != nil || h.csrf.Validate(r) != nil {
		http.Error(w, "csrf", 403)
		return
	}
	inviteRole := r.FormValue("role")
	if inviteRole != "viewer" && inviteRole != "member" && inviteRole != "admin" {
		http.Error(w, "invalid role", 422)
		return
	}
	maxUses, _ := strconv.Atoi(r.FormValue("max_uses"))
	if maxUses != 1 && maxUses != 5 && maxUses != 10 {
		maxUses = 1
	}
	days, _ := strconv.Atoi(r.FormValue("expiry_days"))
	if days != 1 && days != 7 && days != 30 {
		days = 7
	}
	code, _ := ids.Token(32)
	publicID, _ := ids.New("inv")
	err := h.q.CreateInvite(r.Context(), db.CreateInviteInput{PublicID: publicID, CodeHash: db.HashInviteCode(code), Role: inviteRole, ExpiresAt: time.Now().UTC().Add(time.Duration(days) * 24 * time.Hour).Format(time.RFC3339Nano), OrganizationID: org.ID, CreatedBy: u.ID, MaxUses: maxUses})
	invites, _ := h.q.ListInvites(r.Context(), org.ID)
	limits, _ := h.q.GetOrganizationLimits(r.Context(), org.ID)
	key := ""
	if err != nil {
		key = "invite.error.limit"
		code = ""
	}
	web.Render(w, r, map[bool]int{true: 422, false: 200}[err != nil], templates.OrganizationInvites(h.cfg.InstanceName, u, org, invites, limits, code, key))
}
func (h *Handler) RevokeInvite(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFromContext(r.Context())
	org, role, instanceOwner, ok := h.authorize(r, u.ID, true)
	if !ok || !(role == "owner" || role == "admin" || instanceOwner) {
		h.forbidden(w, r)
		return
	}
	if r.ParseForm() != nil || h.csrf.Validate(r) != nil {
		http.Error(w, "csrf", 403)
		return
	}
	_ = h.q.RevokeInvite(r.Context(), chi.URLParam(r, "invitePublicID"), org.ID, u.ID)
	http.Redirect(w, r, "/app/orgs/"+org.PublicID+"/invites", 303)
}

func (h *Handler) JoinGet(w http.ResponseWriter, r *http.Request) {
	token, _ := h.csrf.Token(w, r)
	r = r.WithContext(templates.WithCSRF(r.Context(), token))
	code := chi.URLParam(r, "inviteCode")
	invite, err := h.q.GetInviteByCode(r.Context(), code)
	valid := err == nil && !invite.RevokedAt.Valid && invite.ExpiresAt > db.Now() && invite.UsedCount < invite.MaxUses
	u, logged := auth.UserFromContext(r.Context())
	settings, _ := h.q.Settings(r.Context())
	web.Render(w, r, map[bool]int{true: 200, false: 410}[valid], templates.JoinInvite(h.cfg.InstanceName, userPtr(u, logged), invite, code, valid, settings["registration_enabled"] == "true" || settings["invite_registration_enabled"] == "true", ""))
}
func (h *Handler) JoinPost(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.UserFromContext(r.Context())
	if !ok {
		http.Redirect(w, r, "/login", 303)
		return
	}
	if r.ParseForm() != nil || h.csrf.Validate(r) != nil {
		http.Error(w, "csrf", 403)
		return
	}
	code := chi.URLParam(r, "inviteCode")
	invite, err := h.q.GetInviteByCode(r.Context(), code)
	if err != nil {
		http.Error(w, "invite unavailable", 410)
		return
	}
	err = h.q.AcceptInvite(r.Context(), code, u.ID)
	if errors.Is(err, db.ErrAlreadyMember) {
		http.Redirect(w, r, "/app/orgs/"+invite.OrganizationPublicID, 303)
		return
	}
	if err != nil {
		web.Render(w, r, 422, templates.JoinInvite(h.cfg.InstanceName, &u, invite, code, false, false, "invite.error.unavailable"))
		return
	}
	http.Redirect(w, r, "/app/orgs/"+invite.OrganizationPublicID, 303)
}

func (h *Handler) authorize(r *http.Request, userID int64, manage bool) (db.Organization, string, bool, bool) {
	org, err := h.q.GetOrganizationByPublicID(r.Context(), chi.URLParam(r, "orgPublicID"))
	if err != nil || org.DisabledAt.Valid {
		return org, "", false, false
	}
	owner, _ := h.permissions.IsInstanceOwner(r.Context(), userID)
	role, err := h.q.OrganizationRole(r.Context(), userID, org.ID)
	if owner {
		if err != nil {
			role = "owner"
		}
		return org, role, true, true
	}
	if err != nil {
		return org, "", false, false
	}
	if manage && role != "owner" && role != "admin" {
		return org, role, false, false
	}
	return org, role, false, true
}
func (h *Handler) forbidden(w http.ResponseWriter, r *http.Request) { http.Error(w, "forbidden", 403) }
func (h *Handler) defaultLimits(r *http.Request) db.DefaultLimits {
	settings, err := h.q.Settings(r.Context())
	if err != nil {
		return db.DefaultLimits{MaxOrganizationsPerUser: h.cfg.DefaultMaxOrganizationsPerUser, MaxCampaignsPerOrg: h.cfg.DefaultMaxCampaignsPerOrg, MaxMembersPerOrg: h.cfg.DefaultMaxMembersPerOrg, MaxActiveInvitesPerOrg: h.cfg.DefaultMaxActiveInvitesPerOrg, MaxMonthlyVisitsPerOrg: h.cfg.DefaultMaxMonthlyVisitsPerOrg, MaxMonthlySubmissionsPerOrg: h.cfg.DefaultMaxMonthlySubmissionsPerOrg}
	}
	value := func(key string, fallback int) int {
		parsed, err := strconv.Atoi(settings[key])
		if err != nil {
			return fallback
		}
		return parsed
	}
	return db.DefaultLimits{
		MaxOrganizationsPerUser:     value("default_max_organizations_per_user", h.cfg.DefaultMaxOrganizationsPerUser),
		MaxCampaignsPerOrg:          value("default_max_campaigns_per_org", h.cfg.DefaultMaxCampaignsPerOrg),
		MaxMembersPerOrg:            value("default_max_members_per_org", h.cfg.DefaultMaxMembersPerOrg),
		MaxActiveInvitesPerOrg:      value("default_max_active_invites_per_org", h.cfg.DefaultMaxActiveInvitesPerOrg),
		MaxMonthlyVisitsPerOrg:      value("default_max_monthly_visits_per_org", h.cfg.DefaultMaxMonthlyVisitsPerOrg),
		MaxMonthlySubmissionsPerOrg: value("default_max_monthly_submissions_per_org", h.cfg.DefaultMaxMonthlySubmissionsPerOrg),
	}
}
func slugify(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	return strings.Trim(slugCleaner.ReplaceAllString(v, "-"), "-")
}
func userPtr(u db.User, ok bool) *db.User {
	if !ok {
		return nil
	}
	return &u
}

var _ = sql.ErrNoRows
var _ = fmt.Sprint
