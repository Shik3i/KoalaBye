package templates

import (
	"context"
	"strings"

	"github.com/koalastuff/koalabye/internal/i18n"
	"github.com/koalastuff/koalabye/internal/version"
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

func repositoryURL(ctx context.Context) string {
	if sourceURL := instanceSourceURL(ctx); sourceURL != "" {
		return sourceURL
	}
	return "https://github.com/Shik3i/KoalaBye"
}

func buildIdentifier() string {
	info := version.Current()
	if commit := strings.TrimSpace(info.Commit); commit != "" && commit != "unknown" {
		if len(commit) > 12 {
			return commit[:12]
		}
		return commit
	}
	if release := strings.TrimSpace(info.Version); release != "" {
		return release
	}
	return "dev"
}

func instanceAdmin(ctx context.Context) bool {
	allowed, _ := ctx.Value(instanceAdminContextKey{}).(bool)
	return allowed
}
