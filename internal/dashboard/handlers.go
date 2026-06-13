package dashboard

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/permissions"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
)

type Handler struct {
	cfg         config.Config
	queries     *db.Querier
	permissions *permissions.Service
}

func (h *Handler) Search(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	organizations, err := h.queries.ListOrganizationsForUser(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "could not search organizations", http.StatusInternalServerError)
		return
	}
	campaigns, err := h.queries.ListAllCampaignsForUser(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "could not search campaigns", http.StatusInternalServerError)
		return
	}
	if query != "" {
		needle := strings.ToLower(query)
		organizations = filterOrganizations(organizations, needle)
		campaigns = filterCampaigns(campaigns, needle)
	}
	web.Render(w, r, http.StatusOK, templates.GlobalSearch(h.cfg.InstanceName, user, query, organizations, campaigns))
}

func filterOrganizations(items []db.Organization, query string) []db.Organization {
	out := make([]db.Organization, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name+" "+item.Slug), query) {
			out = append(out, item)
		}
	}
	return out
}

func filterCampaigns(items []db.Campaign, query string) []db.Campaign {
	out := make([]db.Campaign, 0, len(items))
	for _, item := range items {
		if strings.Contains(strings.ToLower(item.Name+" "+item.Slug+" "+item.OrganizationName), query) {
			out = append(out, item)
		}
	}
	return out
}

func New(cfg config.Config, queries *db.Querier, permissionService *permissions.Service) *Handler {
	return &Handler{cfg: cfg, queries: queries, permissions: permissionService}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	user, _ := auth.UserFromContext(r.Context())
	organizations, err := h.queries.ListOrganizationsForUser(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "could not load dashboard", http.StatusInternalServerError)
		return
	}
	campaigns, err := h.queries.ListAllCampaignsForUser(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "could not load campaigns", http.StatusInternalServerError)
		return
	}
	isOwner, err := h.permissions.IsInstanceOwner(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "could not check permissions", http.StatusInternalServerError)
		return
	}
	settings, _ := h.queries.Settings(r.Context())
	limit, _ := strconv.Atoi(settings["default_max_organizations_per_user"])
	count, _ := h.queries.CountOrganizationsCreatedByUser(r.Context(), user.ID)
	web.Render(w, r, http.StatusOK, templates.Dashboard(h.cfg.InstanceName, user, organizations, campaigns, isOwner, count < limit))
}
