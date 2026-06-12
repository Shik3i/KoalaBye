package auth

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/koalastuff/koalabye/internal/audit"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
)

type Handler struct {
	cfg      config.Config
	queries  *db.Querier
	sessions *SessionManager
	csrf     *CSRF
	audit    *audit.Logger
	limiter  *LoginLimiter
}

func NewHandler(cfg config.Config, queries *db.Querier, sessions *SessionManager, csrf *CSRF, auditLogger *audit.Logger) *Handler {
	return &Handler{cfg: cfg, queries: queries, sessions: sessions, csrf: csrf, audit: auditLogger, limiter: NewLoginLimiter()}
}

func (h *Handler) LoginGet(w http.ResponseWriter, r *http.Request) {
	if _, err := h.sessions.CurrentUser(r.Context(), r); err == nil {
		http.Redirect(w, r, "/app", http.StatusSeeOther)
		return
	}
	token, _ := h.csrf.Token(w, r)
	web.Render(w, r, http.StatusOK, templates.Login(h.cfg.InstanceName, token, ""))
}

func (h *Handler) LoginPost(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil || h.csrf.Validate(r) != nil {
		h.loginError(w, r, "Your form expired. Please try again.")
		return
	}
	username := db.NormalizeUsername(r.FormValue("username"))
	if !h.limiter.Allow(username) {
		h.loginError(w, r, "Too many attempts. Please wait a few minutes.")
		return
	}
	user, err := h.queries.GetUserByNormalizedUsername(r.Context(), username)
	if err != nil {
		if err != sql.ErrNoRows {
			h.loginError(w, r, "Login is temporarily unavailable.")
			return
		}
		h.audit.Record(r.Context(), nil, "login_failure", "username", username)
		h.loginError(w, r, "Invalid username or password.")
		return
	}
	valid, err := VerifyPassword(user.PasswordHash, r.FormValue("password"))
	if err != nil || !valid || user.DisabledAt.Valid {
		h.audit.Record(r.Context(), user.ID, "login_failure", "user", user.PublicID)
		h.loginError(w, r, "Invalid username or password.")
		return
	}
	if err := h.sessions.Start(r.Context(), w, user.ID); err != nil {
		h.loginError(w, r, "Login is temporarily unavailable.")
		return
	}
	h.limiter.Success(username)
	h.audit.Record(r.Context(), user.ID, "login_success", "user", user.PublicID)
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

func (h *Handler) LogoutPost(w http.ResponseWriter, r *http.Request) {
	user, _ := UserFromContext(r.Context())
	if err := r.ParseForm(); err != nil || h.csrf.Validate(r) != nil {
		http.Error(w, "invalid CSRF token", http.StatusForbidden)
		return
	}
	if err := h.sessions.End(r.Context(), w, r); err != nil {
		http.Error(w, "logout failed", http.StatusInternalServerError)
		return
	}
	h.audit.Record(r.Context(), user.ID, "logout", "user", user.PublicID)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handler) loginError(w http.ResponseWriter, r *http.Request, message string) {
	token, _ := h.csrf.Token(w, r)
	web.Render(w, r, http.StatusUnprocessableEntity, templates.Login(h.cfg.InstanceName, token, strings.TrimSpace(message)))
}
