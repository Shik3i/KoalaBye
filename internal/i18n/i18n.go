package i18n

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Locale string

const (
	English Locale = "en"
	German  Locale = "de"
	Spanish Locale = "es"

	DefaultLocale    = English
	LanguageCookie   = "koalabye_lang"
	languageLifetime = 365 * 24 * time.Hour
)

var Supported = []Locale{English, German, Spanish}
var LegalSupported = []Locale{English, German}

type Language struct {
	Code        Locale
	EnglishName string
	NativeName  string
	FlagAsset   string
	Enabled     bool
}

var Languages = []Language{
	{Code: English, EnglishName: "English", NativeName: "English", FlagAsset: "/assets/flags/gb.svg", Enabled: true},
	{Code: German, EnglishName: "German", NativeName: "Deutsch", FlagAsset: "/assets/flags/de.svg", Enabled: true},
	{Code: Spanish, EnglishName: "Spanish", NativeName: "Español", FlagAsset: "/assets/flags/es.svg", Enabled: true},
}

func EnabledLanguages() []Language {
	languages := make([]Language, 0, len(Languages))
	for _, language := range Languages {
		if language.Enabled {
			languages = append(languages, language)
		}
	}
	return languages
}

func LanguageByCode(code Locale) Language {
	for _, language := range Languages {
		if language.Code == code {
			return language
		}
	}
	return Languages[0]
}

var publicContextValuePattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

var publicContextKeys = []string{
	"app_version",
	"extension_version",
	"platform",
	"source",
	"channel",
	"utm_source",
	"utm_medium",
	"utm_campaign",
	"utm_content",
	"utm_term",
}

//go:embed locales/*.json
var localeFS embed.FS

type Catalog struct {
	messages map[Locale]map[string]string
}

func Load() (*Catalog, error) {
	catalog := &Catalog{messages: make(map[Locale]map[string]string, len(Supported))}
	for _, locale := range Supported {
		data, err := localeFS.ReadFile("locales/" + string(locale) + ".json")
		if err != nil {
			return nil, fmt.Errorf("read %s locale: %w", locale, err)
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			return nil, fmt.Errorf("parse %s locale: %w", locale, err)
		}
		catalog.messages[locale] = messages
	}
	if err := catalog.ValidateParity(); err != nil {
		return nil, err
	}
	return catalog, nil
}

func (c *Catalog) Translate(locale Locale, key string, args ...any) string {
	locale = Normalize(string(locale))
	message, ok := c.messages[locale][key]
	if !ok {
		message, ok = c.messages[DefaultLocale][key]
	}
	if !ok {
		return "[" + "missing:" + key + "]"
	}
	if len(args) > 0 {
		return fmt.Sprintf(message, args...)
	}
	return message
}

func (c *Catalog) ValidateParity() error {
	baseline := c.messages[DefaultLocale]
	for _, locale := range Supported {
		for key := range baseline {
			if _, ok := c.messages[locale][key]; !ok {
				return fmt.Errorf("locale %s is missing translation key %s", locale, key)
			}
		}
		for key := range c.messages[locale] {
			if _, ok := baseline[key]; !ok {
				return fmt.Errorf("locale %s has unknown translation key %s", locale, key)
			}
		}
	}
	return nil
}

func (c *Catalog) Keys() []string {
	keys := make([]string, 0, len(c.messages[DefaultLocale]))
	for key := range c.messages[DefaultLocale] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func Normalize(value string) Locale {
	value = strings.ToLower(strings.TrimSpace(value))
	if separator := strings.IndexAny(value, "-_"); separator >= 0 {
		value = value[:separator]
	}
	for _, locale := range Supported {
		if value == string(locale) {
			return locale
		}
	}
	return DefaultLocale
}

func IsSupported(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	for _, locale := range Supported {
		if normalized == string(locale) {
			return true
		}
	}
	return false
}

func IsLegalSupported(locale Locale) bool {
	return locale == English || locale == German
}

type contextKey struct{}

type RequestLocale struct {
	Locale            Locale
	RequestedLocale   Locale
	LegalFallbackUsed bool
	SwitchURLs        map[Locale]string
	catalog           *Catalog
}

func WithLocale(ctx context.Context, locale RequestLocale) context.Context {
	return context.WithValue(ctx, contextKey{}, locale)
}

func FromContext(ctx context.Context) RequestLocale {
	locale, ok := ctx.Value(contextKey{}).(RequestLocale)
	if !ok {
		return RequestLocale{Locale: DefaultLocale, RequestedLocale: DefaultLocale}
	}
	return locale
}

func T(ctx context.Context, key string, args ...any) string {
	current := FromContext(ctx)
	if current.catalog == nil {
		return "[" + "missing:" + key + "]"
	}
	return current.catalog.Translate(current.Locale, key, args...)
}

func Middleware(catalog *Catalog, secureCookies bool) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			locale := detect(r)
			if raw := r.URL.Query().Get("lang"); raw != "" {
				locale = Normalize(raw)
				if !IsPublicCampaignPath(r.URL.Path) {
					http.SetCookie(w, &http.Cookie{
						Name: LanguageCookie, Value: string(locale), Path: "/",
						Secure: secureCookies, SameSite: http.SameSiteLaxMode,
						MaxAge: int(languageLifetime.Seconds()), Expires: time.Now().Add(languageLifetime),
					})
				}
			}
			switchURLs := make(map[Locale]string, len(Supported))
			for _, supportedLocale := range Supported {
				switchURLs[supportedLocale] = SwitchURL(r, supportedLocale)
			}
			requestLocale := RequestLocale{
				Locale: locale, RequestedLocale: locale, SwitchURLs: switchURLs, catalog: catalog,
			}
			next.ServeHTTP(w, r.WithContext(WithLocale(r.Context(), requestLocale)))
		})
	}
}

func PublicCampaignContext(ctx context.Context, r *http.Request, defaultLocale string) context.Context {
	current := FromContext(ctx)
	if raw := r.URL.Query().Get("lang"); raw != "" {
		current.Locale = Normalize(raw)
	} else {
		current.Locale = Normalize(defaultLocale)
	}
	current.RequestedLocale = current.Locale
	current.LegalFallbackUsed = false
	current.SwitchURLs = make(map[Locale]string, len(Supported))
	for _, locale := range Supported {
		query := safePublicCampaignQuery(r.URL.Query())
		query.Set("lang", string(locale))
		current.SwitchURLs[locale] = r.URL.Path + "?" + query.Encode()
	}
	return WithLocale(ctx, current)
}

func safePublicCampaignQuery(values url.Values) url.Values {
	safe := make(url.Values)
	for _, key := range publicContextKeys {
		value := strings.TrimSpace(values.Get(key))
		lower := strings.ToLower(value)
		if !publicContextValuePattern.MatchString(value) ||
			strings.Contains(lower, "javascript:") ||
			strings.Contains(lower, "data:") ||
			strings.Contains(lower, "vbscript:") ||
			strings.Contains(lower, "://") {
			continue
		}
		safe.Set(key, value)
	}
	return safe
}

func IsPublicCampaignPath(path string) bool {
	return strings.HasPrefix(path, "/c/") || strings.HasPrefix(path, "/u/")
}

func detect(r *http.Request) Locale {
	if raw := r.URL.Query().Get("lang"); raw != "" {
		return Normalize(raw)
	}
	if cookie, err := r.Cookie(LanguageCookie); err == nil && IsSupported(cookie.Value) {
		return Normalize(cookie.Value)
	}
	for _, part := range strings.Split(r.Header.Get("Accept-Language"), ",") {
		language := strings.TrimSpace(strings.SplitN(part, ";", 2)[0])
		if language == "" || language == "*" {
			continue
		}
		normalized := Normalize(language)
		base := strings.ToLower(strings.SplitN(language, "-", 2)[0])
		if IsSupported(base) {
			return normalized
		}
	}
	return DefaultLocale
}

func LegalContext(ctx context.Context) context.Context {
	current := FromContext(ctx)
	if IsLegalSupported(current.Locale) {
		return ctx
	}
	current.RequestedLocale = current.Locale
	current.Locale = English
	current.LegalFallbackUsed = true
	return WithLocale(ctx, current)
}

func SwitchURL(r *http.Request, locale Locale) string {
	query := cloneValues(r.URL.Query())
	query.Set("lang", string(locale))
	path := r.URL.Path
	if path == "" {
		path = "/"
	}
	return path + "?" + query.Encode()
}

func cloneValues(values url.Values) url.Values {
	cloned := make(url.Values, len(values))
	for key, value := range values {
		cloned[key] = append([]string(nil), value...)
	}
	return cloned
}
