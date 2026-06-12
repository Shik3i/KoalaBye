package setup

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/i18n"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,40}$`)
var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

type Handler struct {
	cfg      config.Config
	queries  *db.Querier
	sessions *auth.SessionManager
	csrf     *auth.CSRF
	catalog  *i18n.Catalog
}

func New(cfg config.Config, queries *db.Querier, sessions *auth.SessionManager, csrf *auth.CSRF, catalog *i18n.Catalog) *Handler {
	return &Handler{cfg: cfg, queries: queries, sessions: sessions, csrf: csrf, catalog: catalog}
}

func (h *Handler) Required(r *http.Request) (bool, error) {
	count, err := h.queries.CountInstanceOwners(r.Context())
	return count == 0, err
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	required, err := h.Required(r)
	if err != nil {
		http.Error(w, "setup check failed", http.StatusInternalServerError)
		return
	}
	if !required {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	token, err := h.csrf.Token(w, r)
	if err != nil {
		http.Error(w, "could not create security token", http.StatusInternalServerError)
		return
	}
	web.Render(w, r, http.StatusOK, templates.Setup(h.cfg.InstanceName, token, ""))
}

func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	required, err := h.Required(r)
	if err != nil || !required {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	if err := r.ParseForm(); err != nil || h.csrf.Validate(r) != nil {
		h.renderError(w, r, "setup.error.expired")
		return
	}
	displayName := strings.TrimSpace(r.FormValue("display_name"))
	username := strings.TrimSpace(r.FormValue("username"))
	password := r.FormValue("password")
	if displayName == "" || len(displayName) > 80 || !usernamePattern.MatchString(username) {
		h.renderError(w, r, "setup.error.invalid_identity")
		return
	}
	if password != r.FormValue("password_confirm") {
		h.renderError(w, r, "setup.error.password_mismatch")
		return
	}
	user, _, err := h.createOwner(r, username, displayName, password, "first_setup_owner_created", "setup")
	if err != nil {
		if errors.Is(err, db.ErrOwnerExists) {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		h.renderError(w, r, "setup.error.create_failed")
		return
	}
	if h.sessions.Start(r.Context(), w, user.ID) != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

func (h *Handler) Bootstrap(r *http.Request) error {
	if h.cfg.BootstrapUsername == "" {
		return nil
	}
	required, err := h.Required(r)
	if err != nil || !required {
		return err
	}
	displayName := h.cfg.BootstrapDisplayName
	if displayName == "" {
		displayName = h.cfg.BootstrapUsername
	}
	_, _, err = h.createOwner(r, h.cfg.BootstrapUsername, displayName, h.cfg.BootstrapPassword, "bootstrap_owner_created", "environment")
	return err
}

func (h *Handler) createOwner(r *http.Request, username, displayName, password, action, source string) (db.User, db.Organization, error) {
	if !usernamePattern.MatchString(username) {
		return db.User{}, db.Organization{}, errors.New("invalid username")
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return db.User{}, db.Organization{}, err
	}
	slug := slugify(displayName)
	if slug == "" {
		slug = db.NormalizeUsername(username)
	}
	return h.queries.CreateFirstOwner(r.Context(), db.FirstOwnerInput{
		UserPublicID: randomID("usr"), Username: username, UsernameNormalized: db.NormalizeUsername(username),
		DisplayName: displayName, PasswordHash: hash, OrganizationPublicID: randomID("org"),
		OrganizationSlug: slug, OrganizationName: h.catalog.Translate(i18n.FromContext(r.Context()).Locale, "setup.default_org_name", displayName),
		InstanceName: h.cfg.InstanceName, RegistrationEnabled: h.cfg.RegistrationEnabled,
		InviteOnly: h.cfg.InviteOnly, InviteRegistrationEnabled: h.cfg.InviteRegistrationEnabled,
		Limits: db.DefaultLimits{
			MaxOrganizationsPerUser:     h.cfg.DefaultMaxOrganizationsPerUser,
			MaxCampaignsPerOrg:          h.cfg.DefaultMaxCampaignsPerOrg,
			MaxMembersPerOrg:            h.cfg.DefaultMaxMembersPerOrg,
			MaxActiveInvitesPerOrg:      h.cfg.DefaultMaxActiveInvitesPerOrg,
			MaxMonthlyVisitsPerOrg:      h.cfg.DefaultMaxMonthlyVisitsPerOrg,
			MaxMonthlySubmissionsPerOrg: h.cfg.DefaultMaxMonthlySubmissionsPerOrg,
		}, AuditAction: action, AuditSource: source,
	})
}

func (h *Handler) renderError(w http.ResponseWriter, r *http.Request, key string) {
	token, _ := h.csrf.Token(w, r)
	web.Render(w, r, http.StatusUnprocessableEntity, templates.Setup(h.cfg.InstanceName, token, key))
}

func randomID(prefix string) string {
	value := make([]byte, 12)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return prefix + "_" + base64.RawURLEncoding.EncodeToString(value)
}

func slugify(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = slugPattern.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}
