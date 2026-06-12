package app

import (
	"database/sql"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/dashboard"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/instance"
	"github.com/koalastuff/koalabye/internal/setup"
	"github.com/koalastuff/koalabye/internal/web"
	"github.com/koalastuff/koalabye/templates"
	staticassets "github.com/koalastuff/koalabye/web/static"
)

func Routes(
	cfg config.Config,
	database *sql.DB,
	queries *db.Querier,
	sessions *auth.SessionManager,
	csrf *auth.CSRF,
	setupHandler *setup.Handler,
	authHandler *auth.Handler,
	dashboardHandler *dashboard.Handler,
	instanceHandler *instance.Handler,
) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))
	r.Use(securityHeaders)
	r.Use(auth.LoadUser(sessions))

	assets, _ := fs.Sub(staticassets.FS, ".")
	r.Handle("/assets/*", http.StripPrefix("/assets/", http.FileServer(http.FS(assets))))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := database.PingContext(r.Context()); err != nil {
			http.Error(w, "database unavailable", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK\n"))
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		required, err := setupHandler.Required(r)
		if err != nil {
			http.Error(w, "could not check setup state", http.StatusInternalServerError)
			return
		}
		if required {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		if _, ok := auth.UserFromContext(r.Context()); ok {
			http.Redirect(w, r, "/app", http.StatusSeeOther)
			return
		}
		web.Render(w, r, http.StatusOK, templates.Landing(cfg.InstanceName))
	})
	r.Get("/setup", setupHandler.Get)
	r.Post("/setup", setupHandler.Post)
	r.Get("/login", func(w http.ResponseWriter, r *http.Request) {
		required, err := setupHandler.Required(r)
		if err != nil {
			http.Error(w, "could not check setup state", http.StatusInternalServerError)
			return
		}
		if required {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		authHandler.LoginGet(w, r)
	})
	r.Post("/login", authHandler.LoginPost)

	r.Group(func(protected chi.Router) {
		protected.Use(auth.RequireUser(csrf))
		protected.Post("/logout", authHandler.LogoutPost)
		protected.Get("/app", dashboardHandler.Get)
		protected.Get("/instance", instanceHandler.Get)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		web.Render(w, r, http.StatusNotFound, templates.ErrorPage(cfg.InstanceName, http.StatusNotFound, "Page not found", "That page does not exist."))
	})
	return r
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
