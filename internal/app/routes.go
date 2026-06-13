package app

import (
	"database/sql"
	"encoding/json"
	"io/fs"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/campaigns"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/dashboard"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/i18n"
	"github.com/koalastuff/koalabye/internal/instance"
	"github.com/koalastuff/koalabye/internal/organizations"
	"github.com/koalastuff/koalabye/internal/registration"
	"github.com/koalastuff/koalabye/internal/setup"
	"github.com/koalastuff/koalabye/internal/version"
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
	catalog *i18n.Catalog,
	setupHandler *setup.Handler,
	authHandler *auth.Handler,
	dashboardHandler *dashboard.Handler,
	instanceHandler *instance.Handler,
	organizationsHandler *organizations.Handler,
	campaignsHandler *campaigns.Handler,
	registrationHandler *registration.Handler,
) http.Handler {
	r := chi.NewRouter()
	r.Use(chimiddleware.RequestID)
	r.Use(chimiddleware.RealIP)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.Timeout(30 * time.Second))
	r.Use(securityHeaders)
	r.Use(i18n.Middleware(catalog, cfg.SecureCookies))
	r.Use(auth.LoadUser(sessions))
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if settings, err := queries.Settings(ctx); err == nil {
				ctx = templates.WithInstanceSettings(ctx, settings)
			}
			if user, ok := auth.UserFromContext(ctx); ok {
				if allowed, err := queries.UserHasInstanceRole(ctx, user.ID, "instance_owner"); err == nil {
					ctx = templates.WithInstanceAdmin(ctx, allowed)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	})

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
	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(w).Encode(version.Current())
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
	r.Get("/register", registrationHandler.Get)
	r.Post("/register", registrationHandler.Post)
	r.Get("/join/{inviteCode}", organizationsHandler.JoinGet)
	r.Get("/c/{campaignPublicID}", campaignsHandler.PublicByID)
	r.Get("/u/{orgSlug}/{campaignSlug}", campaignsHandler.PublicBySlug)
	r.Post("/c/{campaignPublicID}/submit", campaignsHandler.PublicSubmitByID)
	r.Post("/u/{orgSlug}/{campaignSlug}/submit", campaignsHandler.PublicSubmitBySlug)
	r.With(auth.RequireUser(csrf), auth.ValidatePosts(csrf)).Post("/join/{inviteCode}", organizationsHandler.JoinPost)
	r.Get("/legal/privacy", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(i18n.LegalContext(r.Context()))
		settings, _ := queries.Settings(r.Context())
		web.Render(w, r, http.StatusOK, templates.Legal(cfg.InstanceName, "privacy", settings))
	})
	r.Get("/legal/imprint", func(w http.ResponseWriter, r *http.Request) {
		r = r.WithContext(i18n.LegalContext(r.Context()))
		settings, _ := queries.Settings(r.Context())
		web.Render(w, r, http.StatusOK, templates.Legal(cfg.InstanceName, "imprint", settings))
	})

	r.Group(func(protected chi.Router) {
		protected.Use(func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				required, err := setupHandler.Required(r)
				if err != nil {
					web.Render(w, r, http.StatusInternalServerError, templates.ErrorPage(
						cfg.InstanceName,
						http.StatusInternalServerError,
						i18n.T(r.Context(), "error.internal.title"),
						i18n.T(r.Context(), "error.internal.message"),
					))
					return
				}
				if required {
					http.Redirect(w, r, "/setup", http.StatusSeeOther)
					return
				}
				next.ServeHTTP(w, r)
			})
		})
		protected.Use(auth.RequireUser(csrf))
		protected.Use(auth.ValidatePosts(csrf))
		protected.Post("/logout", authHandler.LogoutPost)
		protected.Get("/app", dashboardHandler.Get)
		protected.Get("/app/orgs", organizationsHandler.List)
		protected.Get("/app/orgs/new", organizationsHandler.New)
		protected.Post("/app/orgs", organizationsHandler.Create)
		protected.Get("/app/orgs/{orgPublicID}", organizationsHandler.View)
		protected.Get("/app/orgs/{orgPublicID}/settings", organizationsHandler.Settings)
		protected.Post("/app/orgs/{orgPublicID}/settings", organizationsHandler.SettingsPost)
		protected.Get("/app/orgs/{orgPublicID}/members", organizationsHandler.Members)
		protected.Post("/app/orgs/{orgPublicID}/members/remove", organizationsHandler.RemoveMember)
		protected.Post("/app/orgs/{orgPublicID}/members/role", organizationsHandler.UpdateMemberRole)
		protected.Get("/app/orgs/{orgPublicID}/invites", organizationsHandler.Invites)
		protected.Post("/app/orgs/{orgPublicID}/invites", organizationsHandler.CreateInvite)
		protected.Post("/app/orgs/{orgPublicID}/invites/{invitePublicID}/revoke", organizationsHandler.RevokeInvite)
		protected.Get("/app/orgs/{orgPublicID}/campaigns", campaignsHandler.List)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/new", campaignsHandler.New)
		protected.Post("/app/orgs/{orgPublicID}/campaigns", campaignsHandler.Create)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}", campaignsHandler.Detail)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/settings", campaignsHandler.Settings)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/settings", campaignsHandler.SettingsPost)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/branding", campaignsHandler.Branding)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/branding", campaignsHandler.BrandingPost)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/privacy", campaignsHandler.Privacy)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/privacy", campaignsHandler.PrivacyPost)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/access", campaignsHandler.Access)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/access", campaignsHandler.AccessPost)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/access/{userPublicID}/remove", campaignsHandler.AccessRemove)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/status", campaignsHandler.Status)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/form", campaignsHandler.Form)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/form/fields", campaignsHandler.FormFieldCreate)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/form/fields/{fieldPublicID}/edit", campaignsHandler.FormFieldEdit)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/form/fields/{fieldPublicID}", campaignsHandler.FormFieldUpdate)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/form/fields/{fieldPublicID}/archive", campaignsHandler.FormFieldArchive)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/form/fields/{fieldPublicID}/move", campaignsHandler.FormFieldMove)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/form/fields/{fieldPublicID}/options", campaignsHandler.FormOptionCreate)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/form/options/{optionPublicID}/archive", campaignsHandler.FormOptionArchive)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/responses", campaignsHandler.Responses)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/responses/{submissionPublicID}", campaignsHandler.ResponseDetail)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/analytics", campaignsHandler.Analytics)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/export/submissions.csv", campaignsHandler.ExportCSV)
		protected.Get("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/export/submissions.json", campaignsHandler.ExportJSON)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/retention/delete-old", campaignsHandler.RetentionDeleteOld)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/responses/delete-all", campaignsHandler.DeleteAllResponses)
		protected.Post("/app/orgs/{orgPublicID}/campaigns/{campaignPublicID}/visits/delete-all", campaignsHandler.DeleteAllVisits)
		protected.Get("/instance", instanceHandler.Get)
		protected.Get("/instance/users", instanceHandler.Users)
		protected.Post("/instance/users/status", instanceHandler.UserStatus)
		protected.Get("/instance/organizations", instanceHandler.Organizations)
		protected.Post("/instance/organizations/status", instanceHandler.OrganizationStatus)
		protected.Get("/instance/organizations/limits", instanceHandler.Limits)
		protected.Post("/instance/organizations/limits", instanceHandler.LimitsPost)
		protected.Get("/instance/campaigns", instanceHandler.Campaigns)
		protected.Post("/instance/campaigns/status", instanceHandler.CampaignStatus)
		protected.Get("/instance/settings", instanceHandler.Settings)
		protected.Post("/instance/settings", instanceHandler.SettingsPost)
		protected.Get("/instance/audit", instanceHandler.Audit)
	})

	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		web.Render(w, r, http.StatusNotFound, templates.ErrorPage(
			cfg.InstanceName,
			http.StatusNotFound,
			i18n.T(r.Context(), "error.not_found.title"),
			i18n.T(r.Context(), "error.not_found.message"),
		))
	})
	return r
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'sha256-bsV5JivYxvGywDAZ22EZJKBFip65Ng9xoJVLbBg7bdo=' 'sha256-+OsIn6RhyCZCUkkvtHxFtP0kU3CGdGeLjDd9Fzqdl3o='; img-src 'self' data:; base-uri 'self'; form-action 'self'; frame-ancestors 'none'")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		next.ServeHTTP(w, r)
	})
}
