package i18n

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func testCatalog(t *testing.T) *Catalog {
	t.Helper()
	catalog, err := Load()
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	return catalog
}

func TestCatalogParityAndFallback(t *testing.T) {
	t.Parallel()
	catalog := testCatalog(t)
	if got := catalog.Translate(German, "auth.login.title"); got != "Anmelden" {
		t.Fatalf("expected German translation, got %q", got)
	}
	if got := catalog.Translate(Locale("fr"), "auth.login.title"); got != "Log in" {
		t.Fatalf("unsupported locale did not fall back to English: %q", got)
	}
	if got := catalog.Translate(English, "not.a.real.key"); got != "[missing:not.a.real.key]" {
		t.Fatalf("missing key was not safely visible: %q", got)
	}
}

func TestLanguageDetection(t *testing.T) {
	t.Parallel()
	catalog := testCatalog(t)
	tests := []struct {
		name           string
		target         string
		acceptLanguage string
		want           Locale
		wantCookie     bool
	}{
		{name: "default", target: "/", want: English},
		{name: "german query", target: "/?lang=de", want: German, wantCookie: true},
		{name: "spanish query", target: "/?lang=es", want: Spanish, wantCookie: true},
		{name: "unsupported query", target: "/?lang=fr", want: English, wantCookie: true},
		{name: "german header", target: "/", acceptLanguage: "de-DE,de;q=0.9,en;q=0.8", want: German},
		{name: "spanish header", target: "/", acceptLanguage: "es-MX,es;q=0.9", want: Spanish},
		{name: "cookie beats header", target: "/", acceptLanguage: "es", want: German},
		{name: "query beats cookie", target: "/?lang=es", acceptLanguage: "de", want: Spanish, wantCookie: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			var got Locale
			handler := Middleware(catalog, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				got = FromContext(r.Context()).Locale
			}))
			request := httptest.NewRequest(http.MethodGet, test.target, nil)
			request.Header.Set("Accept-Language", test.acceptLanguage)
			if test.name == "cookie beats header" || test.name == "query beats cookie" {
				request.AddCookie(&http.Cookie{Name: LanguageCookie, Value: "de"})
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if got != test.want {
				t.Fatalf("expected %s, got %s", test.want, got)
			}
			var foundCookie bool
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == LanguageCookie {
					foundCookie = true
					if cookie.Value != string(test.want) {
						t.Fatalf("expected cookie %s, got %s", test.want, cookie.Value)
					}
				}
			}
			if foundCookie != test.wantCookie {
				t.Fatalf("cookie presence: expected %t, got %t", test.wantCookie, foundCookie)
			}
		})
	}
}

func TestPublicCampaignSwitchURLsKeepOnlySafeContext(t *testing.T) {
	t.Parallel()
	catalog := testCatalog(t)
	request := httptest.NewRequest(http.MethodGet, "/c/camp?lang=de&t=raw-token&platform=chrome&utm_campaign=uninstall&email=person@example.com&source=javascript:alert", nil)
	base := RequestLocale{Locale: German, RequestedLocale: German, catalog: catalog}
	ctx := PublicCampaignContext(WithLocale(request.Context(), base), request, "en")
	switchURL := FromContext(ctx).SwitchURLs[Spanish]
	for _, forbidden := range []string{"raw-token", "email", "person", "javascript"} {
		if strings.Contains(switchURL, forbidden) {
			t.Fatalf("switch URL leaked %q: %s", forbidden, switchURL)
		}
	}
	if !strings.Contains(switchURL, "platform=chrome") || !strings.Contains(switchURL, "utm_campaign=uninstall") || !strings.Contains(switchURL, "lang=es") {
		t.Fatalf("switch URL lost safe context: %s", switchURL)
	}
}
