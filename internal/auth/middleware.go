package auth

import (
	"net/http"

	"github.com/koalastuff/koalabye/templates"
)

func LoadUser(sessions *SessionManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if user, err := sessions.CurrentUser(r.Context(), r); err == nil {
				r = r.WithContext(WithUser(r.Context(), user))
			}
			next.ServeHTTP(w, r)
		})
	}
}

func RequireUser(csrf *CSRF) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, ok := UserFromContext(r.Context()); !ok {
				http.Redirect(w, r, "/login", http.StatusSeeOther)
				return
			}
			token, err := csrf.Token(w, r)
			if err != nil {
				http.Error(w, "could not create security token", http.StatusInternalServerError)
				return
			}
			r = r.WithContext(templates.WithCSRF(r.Context(), token))
			next.ServeHTTP(w, r)
		})
	}
}

func ValidatePosts(csrf *CSRF) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost {
				if err := r.ParseForm(); err != nil || csrf.Validate(r) != nil {
					http.Error(w, "invalid CSRF token", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
