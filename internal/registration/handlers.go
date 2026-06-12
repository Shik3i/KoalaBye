package registration

import (
	"net/http"
	"regexp"
	"strings"

	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/ids"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
)

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{3,40}$`)

type Handler struct {
	cfg      config.Config
	q        *db.Querier
	sessions *auth.SessionManager
	csrf     *auth.CSRF
}

func New(cfg config.Config, q *db.Querier, s *auth.SessionManager, c *auth.CSRF) *Handler {
	return &Handler{cfg: cfg, q: q, sessions: s, csrf: c}
}
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	if _, ok := auth.UserFromContext(r.Context()); ok {
		http.Redirect(w, r, "/app", 303)
		return
	}
	token, _ := h.csrf.Token(w, r)
	r = r.WithContext(templates.WithCSRF(r.Context(), token))
	invite := r.URL.Query().Get("invite")
	allowed := h.allowed(r, invite)
	web.Render(w, r, map[bool]int{true: 200, false: 403}[allowed], templates.Register(h.cfg.InstanceName, invite, allowed, ""))
}
func (h *Handler) Post(w http.ResponseWriter, r *http.Request) {
	if r.ParseForm() != nil || h.csrf.Validate(r) != nil {
		http.Error(w, "csrf", 403)
		return
	}
	invite := r.FormValue("invite_code")
	if !h.allowed(r, invite) {
		web.Render(w, r, 403, templates.Register(h.cfg.InstanceName, invite, false, "register.error.disabled"))
		return
	}
	username := strings.TrimSpace(r.FormValue("username"))
	display := strings.TrimSpace(r.FormValue("display_name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	if !usernamePattern.MatchString(username) || display == "" {
		web.Render(w, r, 422, templates.Register(h.cfg.InstanceName, invite, true, "register.error.invalid"))
		return
	}
	if password != r.FormValue("password_confirm") {
		web.Render(w, r, 422, templates.Register(h.cfg.InstanceName, invite, true, "setup.error.password_mismatch"))
		return
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		web.Render(w, r, 422, templates.Register(h.cfg.InstanceName, invite, true, "register.error.password"))
		return
	}
	publicID, _ := ids.New("usr")
	user, err := h.q.CreateUser(r.Context(), publicID, username, db.NormalizeUsername(username), email, strings.ToLower(email), display, hash)
	if err != nil {
		web.Render(w, r, 422, templates.Register(h.cfg.InstanceName, invite, true, "register.error.exists"))
		return
	}
	if invite != "" {
		if err = h.q.AcceptInvite(r.Context(), invite, user.ID); err != nil {
			_, _ = h.q.RawDB().ExecContext(r.Context(), `DELETE FROM users WHERE id=?`, user.ID)
			web.Render(w, r, 422, templates.Register(h.cfg.InstanceName, invite, true, "invite.error.unavailable"))
			return
		}
	}
	if h.sessions.Start(r.Context(), w, user.ID) != nil {
		http.Redirect(w, r, "/login", 303)
		return
	}
	http.Redirect(w, r, "/app", 303)
}
func (h *Handler) allowed(r *http.Request, invite string) bool {
	settings, err := h.q.Settings(r.Context())
	if err != nil {
		return false
	}
	if invite != "" && settings["invite_registration_enabled"] == "true" {
		i, err := h.q.GetInviteByCode(r.Context(), invite)
		return err == nil && !i.RevokedAt.Valid && i.ExpiresAt > db.Now() && i.UsedCount < i.MaxUses
	}
	return settings["registration_enabled"] == "true" && settings["invite_only"] != "true"
}
