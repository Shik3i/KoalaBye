package templates

import "context"

import (
	"github.com/koalastuff/koalabye/internal/i18n"
	"github.com/koalastuff/koalabye/internal/web"
)

type csrfContextKey struct{}
type instanceSettingsContextKey struct{}
type instanceAdminContextKey struct{}

func WithCSRF(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfContextKey{}, token)
}

func WithInstanceSettings(ctx context.Context, settings map[string]string) context.Context {
	return context.WithValue(ctx, instanceSettingsContextKey{}, settings)
}

func WithInstanceAdmin(ctx context.Context, allowed bool) context.Context {
	return context.WithValue(ctx, instanceAdminContextKey{}, allowed)
}

func csrfFromContext(ctx context.Context) string {
	token, _ := ctx.Value(csrfContextKey{}).(string)
	return token
}

func tr(ctx context.Context, key string, args ...any) string {
	return i18n.T(ctx, key, args...)
}

func flashFromContext(ctx context.Context) (web.Flash, bool) {
	return web.FlashFromContext(ctx)
}

func toastRole(kind string) string {
	if kind == "error" {
		return "alert"
	}
	return "status"
}

func toastLive(kind string) string {
	if kind == "error" {
		return "assertive"
	}
	return "polite"
}

func localeFromContext(ctx context.Context) i18n.RequestLocale {
	return i18n.FromContext(ctx)
}

func languageCurrent(ctx context.Context, locale i18n.Locale) string {
	if i18n.FromContext(ctx).Locale == locale {
		return "page"
	}
	return "false"
}

func supportedLanguages() []i18n.Language {
	return i18n.EnabledLanguages()
}

func currentLanguage(ctx context.Context) i18n.Language {
	return i18n.LanguageByCode(i18n.FromContext(ctx).Locale)
}

func instanceSourceURL(ctx context.Context) string {
	if settings, ok := ctx.Value(instanceSettingsContextKey{}).(map[string]string); ok {
		return settings["instance_source_url"]
	}
	return ""
}

func instanceAdmin(ctx context.Context) bool {
	allowed, _ := ctx.Value(instanceAdminContextKey{}).(bool)
	return allowed
}
