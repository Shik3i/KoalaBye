package dashboard

import (
	"net/http"

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
	isOwner, err := h.permissions.IsInstanceOwner(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "could not check permissions", http.StatusInternalServerError)
		return
	}
	web.Render(w, r, http.StatusOK, templates.Dashboard(h.cfg.InstanceName, user, organizations, isOwner))
}
