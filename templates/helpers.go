package templates

import "context"

import "github.com/koalastuff/koalabye/internal/i18n"

type csrfContextKey struct{}

func WithCSRF(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfContextKey{}, token)
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

func instanceSourceURL(ctx context.Context) string {
	if settings, ok := ctx.Value("settings").(map[string]string); ok {
		return settings["instance_source_url"]
	}
	return ""
}
