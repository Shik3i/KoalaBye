package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/koalastuff/koalabye/internal/auth"
	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/db"
	"github.com/koalastuff/koalabye/internal/i18n"
)

func testApp(t *testing.T) *App {
	t.Helper()
	application, err := New(context.Background(), config.Config{
		BaseURL:       "http://localhost:8080",
		ListenAddr:    ":0",
		DatabasePath:  t.TempDir() + "/test.db",
		Secret:        "test-secret-that-is-longer-than-thirty-two-characters",
		Mode:          "selfhost",
		InviteOnly:    true,
		InstanceName:  "KoalaBye Test",
		SecureCookies: false,
	})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() { application.Close() })
	return application
}

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	if response.Body.String() != "OK\n" {
		t.Fatalf("unexpected body %q", response.Body.String())
	}
}

func TestVersionEndpointIsSafe(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	request := httptest.NewRequest(http.MethodGet, "/version", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", response.Code)
	}
	var payload map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode version response: %v", err)
	}
	for _, key := range []string{"app_name", "version", "commit", "build_date", "go_version"} {
		if payload[key] == "" {
			t.Fatalf("missing %s in version response", key)
		}
	}
	for _, forbidden := range []string{"secret", "database_path", "base_url"} {
		if _, ok := payload[forbidden]; ok {
			t.Fatalf("version response exposed %s", forbidden)
		}
	}
}

func TestTemplatesContainNoKnownExternalTrackingReferences(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		"https://cdn.",
		"https://fonts.googleapis.com",
		"https://www.google-analytics.com",
		"googletagmanager",
		"plausible",
		"cloudflareinsights",
	}
	for _, root := range []string{"../../templates", "../../web/static"} {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			lower := strings.ToLower(string(content))
			for _, value := range forbidden {
				if strings.Contains(lower, value) {
					t.Errorf("%s contains forbidden external reference %q", path, value)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan %s: %v", root, err)
		}
	}
}

func TestSetupDisabledWhenOwnerExists(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	now := db.Now()
	result, err := application.Database.Exec(`
		INSERT INTO users (public_id, username, username_normalized, display_name, password_hash, created_at, updated_at)
		VALUES ('usr_owner', 'owner', 'owner', 'Owner', 'hash', ?, ?)`, now, now)
	if err != nil {
		t.Fatalf("insert owner user: %v", err)
	}
	userID, _ := result.LastInsertId()
	if _, err := application.Database.Exec(`
		INSERT INTO instance_roles (user_id, role, created_at) VALUES (?, 'instance_owner', ?)`,
		userID, now); err != nil {
		t.Fatalf("insert owner role: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/setup", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", response.Code)
	}
	if location := response.Header().Get("Location"); location != "/login" {
		t.Fatalf("expected /login redirect, got %q", location)
	}
}

func TestFirstRunSetupCreatesOwnerAndSession(t *testing.T) {
	t.Parallel()
	application := testApp(t)

	getRequest := httptest.NewRequest(http.MethodGet, "/setup", nil)
	getResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("setup GET: expected 200, got %d", getResponse.Code)
	}
	csrfCookie := getResponse.Result().Cookies()[0]
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(getResponse.Body.String())
	if len(match) != 2 {
		t.Fatal("setup form did not contain a CSRF token")
	}

	form := url.Values{
		"csrf_token":       {match[1]},
		"display_name":     {"Test Owner"},
		"username":         {"TestOwner"},
		"password":         {"a sufficiently long password"},
		"password_confirm": {"a sufficiently long password"},
	}
	postRequest := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	postRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRequest.AddCookie(csrfCookie)
	postResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(postResponse, postRequest)
	if postResponse.Code != http.StatusSeeOther {
		t.Fatalf("setup POST: expected 303, got %d: %s", postResponse.Code, postResponse.Body.String())
	}
	if location := postResponse.Header().Get("Location"); location != "/app" {
		t.Fatalf("expected /app redirect, got %q", location)
	}
	var ownerCount int
	if err := application.Database.QueryRow(`
		SELECT COUNT(*) FROM instance_roles WHERE role = 'instance_owner' AND revoked_at IS NULL`).Scan(&ownerCount); err != nil {
		t.Fatalf("count owners: %v", err)
	}
	if ownerCount != 1 {
		t.Fatalf("expected one owner, got %d", ownerCount)
	}
	var organizationName string
	if err := application.Database.QueryRow(`SELECT name FROM organizations LIMIT 1`).Scan(&organizationName); err != nil {
		t.Fatalf("read default organization: %v", err)
	}
	if organizationName != "Test Owner Team" {
		t.Fatalf("unexpected default organization name %q", organizationName)
	}
	var sessionHash string
	if err := application.Database.QueryRow(`SELECT session_hash FROM sessions LIMIT 1`).Scan(&sessionHash); err != nil {
		t.Fatalf("read session: %v", err)
	}
	sessionCookies := postResponse.Result().Cookies()
	var rawToken string
	for _, cookie := range sessionCookies {
		if cookie.Name == "koalabye_session" {
			rawToken = cookie.Value
		}
	}
	if rawToken == "" {
		t.Fatal("setup did not issue a session cookie")
	}
	if sessionHash == rawToken {
		t.Fatal("database stored the raw session token")
	}

	secondGet := httptest.NewRequest(http.MethodGet, "/setup", nil)
	secondResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(secondResponse, secondGet)
	if secondResponse.Code != http.StatusSeeOther || secondResponse.Header().Get("Location") != "/login" {
		t.Fatalf("setup remained available after owner creation")
	}
}

func seedOwner(t *testing.T, application *App) (db.User, string) {
	t.Helper()
	const password = "a sufficiently long password"
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash owner password: %v", err)
	}
	queries := db.NewQuerier(application.Database)
	user, _, err := queries.CreateFirstOwner(context.Background(), db.FirstOwnerInput{
		UserPublicID: "usr_owner", Username: "owner", UsernameNormalized: "owner",
		DisplayName: "Owner", PasswordHash: passwordHash, OrganizationPublicID: "org_owner",
		OrganizationSlug: "owner", OrganizationName: "Owner organization",
		InstanceName: "KoalaBye Test", InviteOnly: true, AuditAction: "first_setup_owner_created",
		AuditSource: "test",
	})
	if err != nil {
		t.Fatalf("seed owner: %v", err)
	}
	return user, password
}

func seedNonOwner(t *testing.T, application *App, username, password string) db.User {
	t.Helper()
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("hash user password: %v", err)
	}
	now := db.Now()
	result, err := application.Database.Exec(`
		INSERT INTO users (public_id, username, username_normalized, display_name, password_hash, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"usr_"+username, username, username, "Regular User", passwordHash, now, now)
	if err != nil {
		t.Fatalf("seed non-owner: %v", err)
	}
	id, _ := result.LastInsertId()
	return db.User{ID: id, PublicID: "usr_" + username, Username: username, UsernameNormalized: username, DisplayName: "Regular User", PasswordHash: passwordHash}
}

type loginResult struct {
	session    *http.Cookie
	csrf       *http.Cookie
	csrfToken  string
	statusCode int
	body       string
}

func login(t *testing.T, application *App, username, password string) loginResult {
	t.Helper()
	getRequest := httptest.NewRequest(http.MethodGet, "/login", nil)
	getResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(getResponse, getRequest)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("login GET: expected 200, got %d", getResponse.Code)
	}
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(getResponse.Body.String())
	if len(match) != 2 {
		t.Fatal("login form did not contain a CSRF token")
	}
	csrfCookie := cookieNamed(getResponse.Result().Cookies(), auth.CSRFCookieName)
	if csrfCookie == nil {
		t.Fatal("login form did not issue a CSRF cookie")
	}
	form := url.Values{
		"csrf_token": {match[1]},
		"username":   {username},
		"password":   {password},
	}
	postRequest := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	postRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postRequest.AddCookie(csrfCookie)
	postResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(postResponse, postRequest)
	return loginResult{
		session: cookieNamed(postResponse.Result().Cookies(), auth.SessionCookieName),
		csrf:    csrfCookie, csrfToken: match[1], statusCode: postResponse.Code, body: postResponse.Body.String(),
	}
}

func cookieNamed(cookies []*http.Cookie, name string) *http.Cookie {
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}

func TestLoginSuccessFailureAndLogoutRevocation(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	user, password := seedOwner(t, application)

	failed := login(t, application, user.Username, "wrong password")
	if failed.statusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid login: expected 422, got %d", failed.statusCode)
	}
	if failed.session != nil {
		t.Fatal("invalid login issued a session")
	}
	if !strings.Contains(failed.body, "Invalid username or password.") {
		t.Fatalf("invalid login did not render a safe error: %s", failed.body)
	}

	success := login(t, application, user.Username, password)
	if success.statusCode != http.StatusSeeOther || success.session == nil {
		t.Fatalf("valid login failed: status=%d session=%v body=%s", success.statusCode, success.session, success.body)
	}
	var storedHash string
	if err := application.Database.QueryRow(`SELECT session_hash FROM sessions ORDER BY id DESC LIMIT 1`).Scan(&storedHash); err != nil {
		t.Fatalf("read stored session: %v", err)
	}
	if storedHash == success.session.Value {
		t.Fatal("raw session token was stored")
	}

	form := url.Values{"csrf_token": {success.csrfToken}}
	logoutRequest := httptest.NewRequest(http.MethodPost, "/logout", strings.NewReader(form.Encode()))
	logoutRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	logoutRequest.AddCookie(success.csrf)
	logoutRequest.AddCookie(success.session)
	logoutResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(logoutResponse, logoutRequest)
	if logoutResponse.Code != http.StatusSeeOther || logoutResponse.Header().Get("Location") != "/login" {
		t.Fatalf("logout failed: status=%d location=%q", logoutResponse.Code, logoutResponse.Header().Get("Location"))
	}
	var revokedAt any
	if err := application.Database.QueryRow(`SELECT revoked_at FROM sessions WHERE session_hash = ?`, storedHash).Scan(&revokedAt); err != nil {
		t.Fatalf("read revoked session: %v", err)
	}
	if revokedAt == nil {
		t.Fatal("logout did not revoke the session")
	}
}

func TestInstancePermissions(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, ownerPassword := seedOwner(t, application)
	const memberPassword = "another sufficiently long password"
	member := seedNonOwner(t, application, "member", memberPassword)

	ownerLogin := login(t, application, owner.Username, ownerPassword)
	ownerRequest := httptest.NewRequest(http.MethodGet, "/instance", nil)
	ownerRequest.AddCookie(ownerLogin.session)
	ownerResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(ownerResponse, ownerRequest)
	if ownerResponse.Code != http.StatusOK {
		t.Fatalf("owner access: expected 200, got %d", ownerResponse.Code)
	}

	memberLogin := login(t, application, member.Username, memberPassword)
	memberRequest := httptest.NewRequest(http.MethodGet, "/instance", nil)
	memberRequest.AddCookie(memberLogin.session)
	memberResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(memberResponse, memberRequest)
	if memberResponse.Code != http.StatusForbidden {
		t.Fatalf("non-owner access: expected 403, got %d", memberResponse.Code)
	}

	anonymousRequest := httptest.NewRequest(http.MethodGet, "/instance", nil)
	anonymousResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(anonymousResponse, anonymousRequest)
	if anonymousResponse.Code != http.StatusSeeOther || anonymousResponse.Header().Get("Location") != "/login" {
		t.Fatalf("anonymous access did not redirect to login")
	}
}

func TestProtectedRoutesRedirectToSetupBeforeFirstOwner(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	request := httptest.NewRequest(http.MethodGet, "/app", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/setup" {
		t.Fatalf("expected setup redirect, got status=%d location=%q", response.Code, response.Header().Get("Location"))
	}
}

func TestI18nRenderingAndLanguageCookie(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	tests := []struct {
		target   string
		header   string
		contains string
		lang     string
	}{
		{target: "/setup", contains: "Create the Instance Owner", lang: "en"},
		{target: "/setup?lang=de", contains: "Instanzinhaber erstellen", lang: "de"},
		{target: "/setup?lang=es", contains: "Crear al propietario de la instancia", lang: "es"},
		{target: "/setup?lang=fr", contains: "Create the Instance Owner", lang: "en"},
		{target: "/setup", header: "de-DE,de;q=0.9", contains: "Instanzinhaber erstellen", lang: "de"},
		{target: "/setup", header: "es-ES,es;q=0.9", contains: "Crear al propietario de la instancia", lang: "es"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(http.MethodGet, test.target, nil)
		request.Header.Set("Accept-Language", test.header)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if !strings.Contains(response.Body.String(), test.contains) {
			t.Fatalf("%s did not contain %q", test.target, test.contains)
		}
		if !strings.Contains(response.Body.String(), `<html lang="`+test.lang+`"`) {
			t.Errorf("expected lang=%q, got body: %s", test.lang, response.Body.String())
		}
		if strings.Contains(test.target, "lang=") {
			cookie := cookieNamed(response.Result().Cookies(), i18n.LanguageCookie)
			if cookie == nil || cookie.Value != test.lang || cookie.SameSite != http.SameSiteLaxMode || cookie.HttpOnly {
				t.Fatalf("%s did not set the expected language cookie", test.target)
			}
		}
	}
}

func TestLegalSpanishFallsBackToEnglish(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	request := httptest.NewRequest(http.MethodGet, "/legal/privacy?lang=es", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	body := response.Body.String()
	if !strings.Contains(body, `<html lang="en"`) || !strings.Contains(body, "English is shown as the fallback") {
		t.Fatalf("expected english fallback, got %s", body)
	}
	for _, expected := range []string{
		`class="site-footer"`,
		`href="https://github.com/Shik3i/KoalaBye"`,
		"Build",
		"admin@koalastuff.net",
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("legal page missing %q: %s", expected, body)
		}
	}
}

func TestSecurityHeadersAssetsAndNoExternalCDN(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	request := httptest.NewRequest(http.MethodGet, "/setup", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	for header, want := range map[string]string{
		"X-Content-Type-Options":    "nosniff",
		"Referrer-Policy":           "no-referrer",
		"X-Frame-Options":           "DENY",
		"Strict-Transport-Security": "max-age=63072000; includeSubDomains",
	} {
		if got := response.Header().Get(header); got != want {
			t.Fatalf("%s: expected %q, got %q", header, want, got)
		}
	}
	csp := response.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "default-src 'self'") || !strings.Contains(csp, "script-src 'self'") || !strings.Contains(csp, "style-src 'self' 'unsafe-inline'") {
		t.Fatalf("missing restrictive CSP: %q", csp)
	}
	body := response.Body.String()
	if strings.Contains(body, `<script src="http`) || strings.Contains(body, `<link rel="stylesheet" href="http`) || strings.Contains(body, `<img src="http`) {
		t.Fatal("rendered HTML contains an external asset URL")
	}
	for _, expected := range []string{
		`rel="manifest" href="/assets/site.webmanifest"`,
		`rel="apple-touch-icon" sizes="180x180"`,
		`class="site-footer__github-icon"`,
		`href="https://github.com/Shik3i/KoalaBye"`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("rendered HTML missing %q", expected)
		}
	}

	assetRequest := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	assetResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(assetResponse, assetRequest)
	if assetResponse.Code != http.StatusOK || !strings.Contains(assetResponse.Body.String(), ":root") {
		t.Fatalf("local CSS asset was not served")
	}
	for _, flag := range []string{"gb.svg", "de.svg", "es.svg"} {
		flagRequest := httptest.NewRequest(http.MethodGet, "/assets/flags/"+flag, nil)
		flagResponse := httptest.NewRecorder()
		application.Handler.ServeHTTP(flagResponse, flagRequest)
		if flagResponse.Code != http.StatusOK || !strings.Contains(flagResponse.Body.String(), "<svg") {
			t.Fatalf("local flag asset %s was not served", flag)
		}
	}
	for _, asset := range []string{
		"site.webmanifest",
		"img/favicon-16x16.png",
		"img/favicon-32x32.png",
		"img/apple-touch-icon.png",
		"img/android-chrome-192x192.png",
		"img/android-chrome-512x512.png",
	} {
		assetRequest := httptest.NewRequest(http.MethodGet, "/assets/"+asset, nil)
		assetResponse := httptest.NewRecorder()
		application.Handler.ServeHTTP(assetResponse, assetRequest)
		if assetResponse.Code != http.StatusOK || assetResponse.Body.Len() == 0 {
			t.Fatalf("local site asset %s was not served", asset)
		}
	}
}

func csrfPage(t *testing.T, application *App, target string, cookies ...*http.Cookie) (*http.Cookie, string, string) {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, target, nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	match := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`).FindStringSubmatch(response.Body.String())
	if len(match) != 2 {
		t.Fatalf("%s did not include CSRF token: status=%d body=%s", target, response.Code, response.Body.String())
	}
	cookie := cookieNamed(response.Result().Cookies(), auth.CSRFCookieName)
	if cookie == nil && len(cookies) > 0 {
		for _, existing := range cookies {
			if existing.Name == auth.CSRFCookieName {
				cookie = existing
			}
		}
	}
	return cookie, match[1], response.Body.String()
}

func formPost(application *App, target string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range cookies {
		if cookie != nil {
			request.AddCookie(cookie)
		}
	}
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	return response
}

func TestRegistrationPoliciesAndOptionalEmail(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	seedOwner(t, application)
	disabledRequest := httptest.NewRequest(http.MethodGet, "/register", nil)
	disabledResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(disabledResponse, disabledRequest)
	if disabledResponse.Code != http.StatusForbidden {
		t.Fatalf("disabled registration status=%d", disabledResponse.Code)
	}
	q := db.NewQuerier(application.Database)
	if err := q.UpdateSettings(context.Background(), map[string]string{"registration_enabled": "true", "invite_only": "false"}, 1); err != nil {
		t.Fatal(err)
	}
	csrfCookie, token, _ := csrfPage(t, application, "/register")
	form := url.Values{"csrf_token": {token}, "display_name": {"New User"}, "username": {"newuser"}, "email": {""}, "password": {"a sufficiently long password"}, "password_confirm": {"a sufficiently long password"}}
	response := formPost(application, "/register", form, csrfCookie)
	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/app" {
		t.Fatalf("registration failed: %d %s", response.Code, response.Body.String())
	}
	var email any
	if err := application.Database.QueryRow(`SELECT email FROM users WHERE username='newuser'`).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if email != nil {
		t.Fatalf("optional email stored unexpectedly: %#v", email)
	}
}

func TestInviteRegistrationFollowsSetting(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, _ := seedOwner(t, application)
	q := db.NewQuerier(application.Database)
	orgs, _ := q.ListOrganizationsForUser(context.Background(), owner.ID)
	org := orgs[0]
	if err := q.CreateInvite(context.Background(), db.CreateInviteInput{PublicID: "inv_register", CodeHash: db.HashInviteCode("register-code"), Role: "viewer", ExpiresAt: time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), OrganizationID: org.ID, CreatedBy: owner.ID, MaxUses: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateSettings(context.Background(), map[string]string{"registration_enabled": "false", "invite_only": "true", "invite_registration_enabled": "false"}, owner.ID); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/register?invite=register-code", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("invite registration ignored disabled setting: %d", response.Code)
	}
	if err := q.UpdateSettings(context.Background(), map[string]string{"invite_registration_enabled": "true"}, owner.ID); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/register?invite=register-code", nil)
	response = httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("invite registration not enabled: %d", response.Code)
	}
}

func TestOrganizationAccessAndDisabledState(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, password := seedOwner(t, application)
	q := db.NewQuerier(application.Database)
	orgs, _ := q.ListOrganizationsForUser(context.Background(), owner.ID)
	org := orgs[0]
	ownerLogin := login(t, application, owner.Username, password)
	request := httptest.NewRequest(http.MethodGet, "/app/orgs/"+org.PublicID+"/settings", nil)
	request.AddCookie(ownerLogin.session)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("owner settings access=%d", response.Code)
	}
	member := seedNonOwner(t, application, "outsider", "another sufficiently long password")
	memberLogin := login(t, application, member.Username, "another sufficiently long password")
	request = httptest.NewRequest(http.MethodGet, "/app/orgs/"+org.PublicID, nil)
	request.AddCookie(memberLogin.session)
	response = httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-member access=%d", response.Code)
	}
	if err := q.SetOrganizationDisabled(context.Background(), org.PublicID, true, owner.ID); err != nil {
		t.Fatal(err)
	}
	request = httptest.NewRequest(http.MethodGet, "/app/orgs/"+org.PublicID, nil)
	request.AddCookie(ownerLogin.session)
	response = httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("disabled org access=%d", response.Code)
	}
}

func TestInvitePermissionsAndCSRF(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, password := seedOwner(t, application)
	q := db.NewQuerier(application.Database)
	orgs, _ := q.ListOrganizationsForUser(context.Background(), owner.ID)
	org := orgs[0]
	ownerLogin := login(t, application, owner.Username, password)
	noCSRF := formPost(application, "/app/orgs/"+org.PublicID+"/invites", url.Values{"role": {"member"}}, ownerLogin.session)
	if noCSRF.Code != http.StatusForbidden {
		t.Fatalf("missing csrf accepted: %d", noCSRF.Code)
	}
	viewer := seedNonOwner(t, application, "viewer", "viewer sufficiently long password")
	if _, err := application.Database.Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id)VALUES(?,?,'viewer',?,?)`, org.ID, viewer.ID, db.Now(), owner.ID); err != nil {
		t.Fatal(err)
	}
	viewerLogin := login(t, application, viewer.Username, "viewer sufficiently long password")
	request := httptest.NewRequest(http.MethodGet, "/app/orgs/"+org.PublicID+"/invites", nil)
	request.AddCookie(viewerLogin.session)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("viewer accessed invites: %d", response.Code)
	}
	csrfCookie, token, _ := csrfPage(t, application, "/app/orgs/"+org.PublicID+"/invites", ownerLogin.session)
	created := formPost(application, "/app/orgs/"+org.PublicID+"/invites", url.Values{"csrf_token": {token}, "role": {"member"}, "max_uses": {"1"}, "expiry_days": {"7"}}, ownerLogin.session, csrfCookie)
	if created.Code != http.StatusOK {
		t.Fatalf("owner invite creation=%d %s", created.Code, created.Body.String())
	}
	match := regexp.MustCompile(`<code>([^<]+)</code>`).FindStringSubmatch(created.Body.String())
	if len(match) != 2 {
		t.Fatal("raw invite not shown once")
	}
	var stored string
	if err := application.Database.QueryRow(`SELECT code_hash FROM invites ORDER BY id DESC LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == match[1] {
		t.Fatal("raw invite stored")
	}
}

func TestInstanceAdminPagesActionsAndAudit(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, password := seedOwner(t, application)
	normal := seedNonOwner(t, application, "normal", "normal sufficiently long password")
	ownerLogin := login(t, application, owner.Username, password)
	for _, path := range []string{"/instance/users", "/instance/organizations", "/instance/settings", "/instance/audit"} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(ownerLogin.session)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s owner status=%d", path, response.Code)
		}
	}
	normalLogin := login(t, application, normal.Username, "normal sufficiently long password")
	request := httptest.NewRequest(http.MethodGet, "/instance/users", nil)
	request.AddCookie(normalLogin.session)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("normal user admin access=%d", response.Code)
	}
	csrfCookie, token, _ := csrfPage(t, application, "/instance/users", ownerLogin.session)
	disabled := formPost(application, "/instance/users/status", url.Values{"csrf_token": {token}, "public_id": {normal.PublicID}, "disabled": {"true"}}, ownerLogin.session, csrfCookie)
	if disabled.Code != http.StatusSeeOther {
		t.Fatalf("disable user=%d", disabled.Code)
	}
	var auditCount int
	if err := application.Database.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='user_disabled' AND target_id=?`, normal.PublicID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("user disable not audited: %d %v", auditCount, err)
	}
	lastOwner := formPost(application, "/instance/users/status", url.Values{"csrf_token": {token}, "public_id": {owner.PublicID}, "disabled": {"true"}}, ownerLogin.session, csrfCookie)
	if lastOwner.Code != http.StatusUnprocessableEntity {
		t.Fatalf("last owner disabled: %d", lastOwner.Code)
	}
}

func TestSharedControlsAdminNavigationAndSourceLink(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, password := seedOwner(t, application)
	normal := seedNonOwner(t, application, "navuser", "navigation user password")

	landing := httptest.NewRecorder()
	application.Handler.ServeHTTP(landing, httptest.NewRequest(http.MethodGet, "/", nil))
	if body := landing.Body.String(); !strings.Contains(body, `<select name="lang"`) || !strings.Contains(body, `data-theme-selector`) || !strings.Contains(body, `/assets/flags/gb.svg`) {
		t.Fatalf("landing is missing shared controls: %s", body)
	}
	if strings.Contains(landing.Body.String(), "English Deutsch Español") {
		t.Fatal("landing still renders a fixed language link row")
	}

	ownerLogin := login(t, application, owner.Username, password)
	ownerRequest := httptest.NewRequest(http.MethodGet, "/app", nil)
	ownerRequest.AddCookie(ownerLogin.session)
	ownerResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(ownerResponse, ownerRequest)
	ownerBody := ownerResponse.Body.String()
	if !strings.Contains(ownerBody, `href="/instance">Admin</a>`) && !strings.Contains(ownerBody, `href="/instance"`) {
		t.Fatalf("owner dashboard is missing admin navigation or shared controls: %s", ownerBody)
	}
	if !strings.Contains(ownerBody, `<details class="nav-menu" open>`) || !strings.Contains(ownerBody, `<select name="lang"`) || !strings.Contains(ownerBody, `data-theme-selector`) {
		t.Fatalf("owner dashboard is missing responsive navigation or shared controls: %s", ownerBody)
	}

	normalLogin := login(t, application, normal.Username, "navigation user password")
	normalRequest := httptest.NewRequest(http.MethodGet, "/app", nil)
	normalRequest.AddCookie(normalLogin.session)
	normalResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(normalResponse, normalRequest)
	if strings.Contains(normalResponse.Body.String(), `href="/instance"`) {
		t.Fatal("regular user saw instance admin navigation")
	}

	q := db.NewQuerier(application.Database)
	sourceURL := "https://github.com/Shik3i/KoalaBye?ref=release&view=source"
	if err := q.UpdateSettings(context.Background(), map[string]string{"instance_source_url": sourceURL}, owner.ID); err != nil {
		t.Fatal(err)
	}
	withSource := httptest.NewRecorder()
	application.Handler.ServeHTTP(withSource, httptest.NewRequest(http.MethodGet, "/", nil))
	if body := withSource.Body.String(); !strings.Contains(body, `https://github.com/Shik3i/KoalaBye?ref=release&amp;view=source`) {
		t.Fatalf("configured source URL was not safely rendered: %s", body)
	}
	if err := q.UpdateSettings(context.Background(), map[string]string{"instance_source_url": ""}, owner.ID); err != nil {
		t.Fatal(err)
	}
	withoutSource := httptest.NewRecorder()
	application.Handler.ServeHTTP(withoutSource, httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Contains(withoutSource.Body.String(), "ref=release&amp;view=source") {
		t.Fatal("empty source URL still rendered a source link")
	}
}

func TestInstanceOwnerUpdatesOrganizationLimitsWithValidationAndAudit(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, password := seedOwner(t, application)
	normal := seedNonOwner(t, application, "limituser", "limit user long password")
	q := db.NewQuerier(application.Database)
	orgs, err := q.ListOrganizationsForUser(context.Background(), owner.ID)
	if err != nil || len(orgs) != 1 {
		t.Fatalf("load owner organization: %v", err)
	}
	org := orgs[0]

	ownerLogin := login(t, application, owner.Username, password)
	csrfCookie, token, detailBody := csrfPage(t, application, "/app/orgs/"+org.PublicID, ownerLogin.session)
	if !strings.Contains(detailBody, "Manage limits in admin") {
		t.Fatal("organization detail did not expose limit management to the instance owner")
	}
	valid := url.Values{
		"csrf_token": {token}, "public_id": {org.PublicID}, "max_campaigns": {"12"},
		"max_members": {"25"}, "max_active_invites": {"30"}, "max_monthly_visits": {"50000"},
		"max_monthly_submissions": {"5000"},
	}
	response := formPost(application, "/instance/organizations/limits", valid, ownerLogin.session, csrfCookie)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("valid limit update failed: %d %s", response.Code, response.Body.String())
	}
	limits, err := q.GetOrganizationLimits(context.Background(), org.ID)
	if err != nil || limits.MaxMembers != 25 || limits.MaxCampaigns != 12 {
		t.Fatalf("limits were not updated: %#v %v", limits, err)
	}
	var auditCount int
	if err := application.Database.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='organization_limits_updated' AND organization_id=?`, org.ID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("limit update was not audited: %d %v", auditCount, err)
	}

	invalid := url.Values{
		"csrf_token": {token}, "public_id": {org.PublicID}, "max_campaigns": {"10001"},
		"max_members": {"0"}, "max_active_invites": {"1"}, "max_monthly_visits": {"1"},
		"max_monthly_submissions": {"1"},
	}
	if rejected := formPost(application, "/instance/organizations/limits", invalid, ownerLogin.session, csrfCookie); rejected.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid limits were accepted: %d", rejected.Code)
	}

	normalLogin := login(t, application, normal.Username, "limit user long password")
	if denied := formPost(application, "/instance/organizations/limits", valid, normalLogin.session, csrfCookie); denied.Code != http.StatusForbidden {
		t.Fatalf("non-admin updated global organization limits: %d", denied.Code)
	}
}

func TestInstanceSettingsRejectNonHTTPSSourceURL(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, password := seedOwner(t, application)
	ownerLogin := login(t, application, owner.Username, password)
	csrfCookie, token, _ := csrfPage(t, application, "/instance/settings", ownerLogin.session)
	form := url.Values{
		"csrf_token": {token}, "instance_name": {"KoalaBye Test"},
		"default_max_organizations_per_user": {"1"}, "default_max_campaigns_per_org": {"3"},
		"default_max_members_per_org": {"5"}, "default_max_active_invites_per_org": {"10"},
		"default_max_monthly_visits_per_org": {"10000"}, "default_max_monthly_submissions_per_org": {"1000"},
		"instance_source_url": {"http://example.com/source"},
	}
	response := formPost(application, "/instance/settings", form, ownerLogin.session, csrfCookie)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("non-HTTPS source URL was accepted: %d", response.Code)
	}
}

func TestNewRoutesRenderGermanAndSpanishWithoutExternalCDN(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	seedOwner(t, application)
	for _, tc := range []struct{ path, text string }{{"/register?lang=de", "öffentliche Registrierung"}, {"/register?lang=es", "registro público"}} {
		request := httptest.NewRequest(http.MethodGet, tc.path, nil)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if !strings.Contains(strings.ToLower(response.Body.String()), strings.ToLower(tc.text)) {
			t.Fatalf("%s missing translation", tc.path)
		}
		body := response.Body.String()
		if strings.Contains(body, `<script src="http`) || strings.Contains(body, `<link rel="stylesheet" href="http`) || strings.Contains(body, `<img src="http`) {
			t.Fatalf("%s contains external asset link", tc.path)
		}
	}
}

func TestCampaignCreationPermissionsQuotaAndCSRF(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, password := seedOwner(t, application)
	q := db.NewQuerier(application.Database)
	orgs, _ := q.ListOrganizationsForUser(context.Background(), owner.ID)
	org := orgs[0]
	ownerLogin := login(t, application, owner.Username, password)
	target := "/app/orgs/" + org.PublicID + "/campaigns"

	noCSRF := formPost(application, target, url.Values{"name": {"No CSRF"}, "slug": {"no-csrf"}}, ownerLogin.session)
	if noCSRF.Code != http.StatusForbidden {
		t.Fatalf("campaign POST without CSRF=%d", noCSRF.Code)
	}
	csrfCookie, token, _ := csrfPage(t, application, target+"/new", ownerLogin.session)
	created := formPost(application, target, url.Values{
		"csrf_token": {token}, "name": {"KoalaSync Chrome"}, "slug": {"koalasync-chrome"},
		"description": {"Extension feedback"}, "public_language_default": {"en"}, "privacy_preset": {"strict"},
	}, ownerLogin.session, csrfCookie)
	if created.Code != http.StatusSeeOther || !regexp.MustCompile(`/campaigns/camp_[A-Za-z0-9_-]+$`).MatchString(created.Header().Get("Location")) {
		t.Fatalf("owner campaign creation=%d location=%q body=%s", created.Code, created.Header().Get("Location"), created.Body.String())
	}

	admin := seedNonOwner(t, application, "campaignadmin", "campaign admin long password")
	viewer := seedNonOwner(t, application, "campaignviewer", "campaign viewer long password")
	outsider := seedNonOwner(t, application, "campaignoutsider", "campaign outsider long password")
	for _, membership := range []struct {
		user db.User
		role string
	}{{admin, "admin"}, {viewer, "viewer"}} {
		if _, err := application.Database.Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id) VALUES(?,?,?,?,?)`, org.ID, membership.user.ID, membership.role, db.Now(), owner.ID); err != nil {
			t.Fatal(err)
		}
	}
	adminLogin := login(t, application, admin.Username, "campaign admin long password")
	adminCSRF, adminToken, _ := csrfPage(t, application, target+"/new", adminLogin.session)
	adminCreated := formPost(application, target, url.Values{
		"csrf_token": {adminToken}, "name": {"Admin Campaign"}, "slug": {"admin-campaign"},
		"public_language_default": {"de"}, "privacy_preset": {"balanced"},
	}, adminLogin.session, adminCSRF)
	if adminCreated.Code != http.StatusSeeOther {
		t.Fatalf("org admin could not create campaign: %d %s", adminCreated.Code, adminCreated.Body.String())
	}
	for _, tc := range []struct {
		user     db.User
		password string
	}{{viewer, "campaign viewer long password"}, {outsider, "campaign outsider long password"}} {
		session := login(t, application, tc.user.Username, tc.password)
		request := httptest.NewRequest(http.MethodGet, target+"/new", nil)
		request.AddCookie(session.session)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("%s create page status=%d", tc.user.Username, response.Code)
		}
	}

	if _, err := application.Database.Exec(`UPDATE organization_limits SET max_campaigns=2 WHERE organization_id=?`, org.ID); err != nil {
		t.Fatal(err)
	}
	_, limitToken, limitBody := csrfPage(t, application, target, ownerLogin.session, csrfCookie)
	if !strings.Contains(limitBody, "safety limit") || strings.Contains(strings.ToLower(limitBody), "upgrade") || strings.Contains(strings.ToLower(limitBody), "paid") {
		t.Fatalf("quota page language is not safety-focused: %s", limitBody)
	}
	limited := formPost(application, target, url.Values{
		"csrf_token": {limitToken}, "name": {"Over Limit"}, "slug": {"over-limit"},
		"public_language_default": {"en"}, "privacy_preset": {"strict"},
	}, ownerLogin.session, csrfCookie)
	if limited.Code != http.StatusUnprocessableEntity || !strings.Contains(limited.Body.String(), "safety limit") {
		t.Fatalf("campaign quota response=%d %s", limited.Code, limited.Body.String())
	}
}

func TestCampaignRolesPrivacyAndArchivedReadOnly(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, password := seedOwner(t, application)
	q := db.NewQuerier(application.Database)
	orgs, _ := q.ListOrganizationsForUser(context.Background(), owner.ID)
	org := orgs[0]
	campaign, err := q.CreateCampaign(context.Background(), db.CreateCampaignInput{PublicID: "camp_http", OrganizationID: org.ID, CreatedBy: owner.ID, Name: "HTTP Campaign", Slug: "http-campaign", Language: "en", PrivacyPreset: "strict"})
	if err != nil {
		t.Fatal(err)
	}
	viewer := seedNonOwner(t, application, "explicitviewer", "explicit viewer long password")
	if _, err = application.Database.Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id) VALUES(?,?,'member',?,?)`, org.ID, viewer.ID, db.Now(), owner.ID); err != nil {
		t.Fatal(err)
	}
	loaded, _ := q.GetCampaignByPublicID(context.Background(), org.PublicID, campaign.PublicID)
	if err = q.SetCampaignMember(context.Background(), loaded, viewer.PublicID, "viewer", owner.ID); err != nil {
		t.Fatal(err)
	}
	viewerLogin := login(t, application, viewer.Username, "explicit viewer long password")
	base := "/app/orgs/" + org.PublicID + "/campaigns/" + campaign.PublicID
	request := httptest.NewRequest(http.MethodGet, base, nil)
	request.AddCookie(viewerLogin.session)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("explicit viewer detail=%d", response.Code)
	}
	for _, path := range []string{base + "/settings", base + "/privacy", base + "/access"} {
		request = httptest.NewRequest(http.MethodGet, path, nil)
		request.AddCookie(viewerLogin.session)
		response = httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden {
			t.Fatalf("viewer accessed %s: %d", path, response.Code)
		}
	}

	ownerLogin := login(t, application, owner.Username, password)
	csrfCookie, token, privacyBody := csrfPage(t, application, base+"/privacy?lang=de", ownerLogin.session)
	if !strings.Contains(privacyBody, "speichert niemals IP-Adressen") || strings.Contains(privacyBody, "https://cdn") {
		t.Fatalf("German privacy page missing policy or contains CDN: %s", privacyBody)
	}
	privacy := formPost(application, base+"/privacy", url.Values{
		"csrf_token": {token}, "collect_install_token": {"on"}, "count_raw_visits": {"on"},
		"count_unique_token_visits": {"on"}, "collect_coarse_browser": {"on"},
		"public_language_default": {"es"}, "show_privacy_notice": {"on"},
	}, ownerLogin.session, csrfCookie)
	if privacy.Code != http.StatusSeeOther {
		t.Fatalf("privacy update=%d %s", privacy.Code, privacy.Body.String())
	}
	var hashToken, coarseBrowser int
	if err := application.Database.QueryRow(`SELECT hash_install_token,collect_coarse_browser FROM campaign_settings WHERE campaign_id=?`, campaign.ID).Scan(&hashToken, &coarseBrowser); err != nil || hashToken != 1 || coarseBrowser != 1 {
		t.Fatalf("privacy settings not safely stored: hash=%d browser=%d err=%v", hashToken, coarseBrowser, err)
	}
	status := formPost(application, base+"/status", url.Values{"csrf_token": {token}, "status": {"archived"}}, ownerLogin.session, csrfCookie)
	if status.Code != http.StatusSeeOther {
		t.Fatalf("archive=%d", status.Code)
	}
	request = httptest.NewRequest(http.MethodGet, base+"/settings", nil)
	request.AddCookie(ownerLogin.session)
	response = httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("archived settings remained writable: %d", response.Code)
	}
}

func TestInstanceCampaignModerationAndTranslations(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, password := seedOwner(t, application)
	q := db.NewQuerier(application.Database)
	orgs, _ := q.ListOrganizationsForUser(context.Background(), owner.ID)
	org := orgs[0]
	campaign, err := q.CreateCampaign(context.Background(), db.CreateCampaignInput{PublicID: "camp_moderate", OrganizationID: org.ID, CreatedBy: owner.ID, Name: "Moderate Me", Slug: "moderate-me", Language: "es", PrivacyPreset: "balanced"})
	if err != nil {
		t.Fatal(err)
	}
	ownerLogin := login(t, application, owner.Username, password)
	csrfCookie, token, body := csrfPage(t, application, "/instance/campaigns?lang=es", ownerLogin.session)
	if !strings.Contains(body, "Últimas 100 campañas") || strings.Contains(body, "https://cdn") {
		t.Fatalf("Spanish campaign admin page invalid")
	}
	disabled := formPost(application, "/instance/campaigns/status", url.Values{"csrf_token": {token}, "public_id": {campaign.PublicID}, "disabled": {"true"}}, ownerLogin.session, csrfCookie)
	if disabled.Code != http.StatusSeeOther {
		t.Fatalf("disable campaign=%d", disabled.Code)
	}
	var disabledAt any
	if err := application.Database.QueryRow(`SELECT disabled_at FROM campaigns WHERE public_id=?`, campaign.PublicID).Scan(&disabledAt); err != nil || disabledAt == nil {
		t.Fatalf("campaign not disabled: %v %v", disabledAt, err)
	}
	var auditCount int
	if err := application.Database.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='campaign_disabled' AND target_id=?`, campaign.PublicID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("campaign disable not audited: %d %v", auditCount, err)
	}
	enabled := formPost(application, "/instance/campaigns/status", url.Values{"csrf_token": {token}, "public_id": {campaign.PublicID}, "disabled": {"false"}}, ownerLogin.session, csrfCookie)
	if enabled.Code != http.StatusSeeOther {
		t.Fatalf("enable campaign=%d", enabled.Code)
	}
	if err := application.Database.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action='campaign_enabled' AND target_id=?`, campaign.PublicID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("campaign enable not audited: %d %v", auditCount, err)
	}
	normal := seedNonOwner(t, application, "campaignnormal", "campaign normal long password")
	normalLogin := login(t, application, normal.Username, "campaign normal long password")
	request := httptest.NewRequest(http.MethodGet, "/instance/campaigns", nil)
	request.AddCookie(normalLogin.session)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("normal user campaign admin access=%d", response.Code)
	}
}

func seedActivePublicCampaign(t *testing.T, application *App, publicID, slug, language string) (db.User, db.Organization, db.Campaign) {
	t.Helper()
	owner, _ := seedOwner(t, application)
	q := db.NewQuerier(application.Database)
	orgs, err := q.ListOrganizationsForUser(context.Background(), owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	org := orgs[0]
	campaign, err := q.CreateCampaign(context.Background(), db.CreateCampaignInput{
		PublicID: publicID, OrganizationID: org.ID, CreatedBy: owner.ID,
		Name: "Public Campaign", Slug: slug, Language: language, PrivacyPreset: "strict",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = application.Database.Exec(`UPDATE campaigns SET status='active',public_link_enabled=1 WHERE id=?`, campaign.ID); err != nil {
		t.Fatal(err)
	}
	loaded, err := q.GetCampaignByPublicID(context.Background(), org.PublicID, campaign.PublicID)
	if err != nil {
		t.Fatal(err)
	}
	return owner, org, loaded
}

func TestPublicCampaignRoutesPrivacyAndVisitCounting(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	_, org, campaign := seedActivePublicCampaign(t, application, "camp_public", "public-campaign", "es")
	if _, err := application.Database.Exec(`UPDATE campaign_settings SET collect_referrer_domain=1,collect_coarse_browser=1,collect_coarse_os=1,collect_url_context=1 WHERE campaign_id=?`, campaign.ID); err != nil {
		t.Fatal(err)
	}
	rawToken := "opaque-install-token"
	request := httptest.NewRequest(http.MethodGet, "/c/"+campaign.PublicID+"?t="+rawToken+"&platform=chrome&utm_campaign=uninstall&email=person@example.com&source=javascript:alert", nil)
	request.Header.Set("Referer", "https://Example.COM/private/path?secret=value")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0) AppleWebKit Chrome/124.0 Safari/537.36")
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `<html lang="es"`) || !strings.Contains(body, "Comentarios opcionales") {
		t.Fatalf("public campaign by ID failed: status=%d body=%s", response.Code, body)
	}
	if len(response.Result().Cookies()) != 0 {
		t.Fatalf("public page set cookies: %#v", response.Result().Cookies())
	}
	if strings.Contains(body, rawToken) || strings.Contains(body, "https://cdn") || strings.Contains(body, "htmx.min.js") || !strings.Contains(body, `src="/assets/app.js`) {
		t.Fatal("public page leaked token or loaded unexpected scripts/external assets")
	}
	mac := hmac.New(sha256.New, []byte(application.Config.Secret))
	_, _ = mac.Write([]byte(rawToken))
	expectedHash := hex.EncodeToString(mac.Sum(nil))
	var storedHash, referrer, browser, os, contextJSON string
	if err := application.Database.QueryRow(`SELECT install_token_hash,referrer_domain,coarse_browser,coarse_os,context_json FROM campaign_visits ORDER BY id LIMIT 1`).Scan(&storedHash, &referrer, &browser, &os, &contextJSON); err != nil {
		t.Fatal(err)
	}
	if storedHash != expectedHash || storedHash == rawToken {
		t.Fatalf("install token was not HMAC-hashed: %q", storedHash)
	}
	if referrer != "example.com" || browser != "Chrome" || os != "Windows" {
		t.Fatalf("coarse metadata incorrect: referrer=%q browser=%q os=%q", referrer, browser, os)
	}
	if contextJSON != `{"platform":"chrome","utm_campaign":"uninstall"}` {
		t.Fatalf("URL context was not minimized: %q", contextJSON)
	}
	var leaked int
	if err := application.Database.QueryRow(`SELECT COUNT(*) FROM campaign_visits WHERE referrer_domain LIKE '%/%' OR coarse_browser LIKE '%Mozilla%' OR coarse_os LIKE '%Mozilla%'`).Scan(&leaked); err != nil || leaked != 0 {
		t.Fatalf("full URL or raw user-agent stored: %d %v", leaked, err)
	}

	for _, target := range []string{
		"/c/" + campaign.PublicID + "?t=" + rawToken + "&lang=de",
		"/u/" + org.Slug + "/" + campaign.Slug + "?t=" + rawToken + "&lang=en",
	} {
		request = httptest.NewRequest(http.MethodGet, target, nil)
		response = httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || strings.Contains(response.Body.String(), rawToken) {
			t.Fatalf("%s failed or leaked token: %d", target, response.Code)
		}
	}
	var rawTotal, uniqueTotal int
	if err := application.Database.QueryRow(`SELECT SUM(counted_as_raw_visit),SUM(counted_as_unique_token_visit) FROM campaign_visits WHERE campaign_id=?`, campaign.ID).Scan(&rawTotal, &uniqueTotal); err != nil {
		t.Fatal(err)
	}
	if rawTotal != 1 || uniqueTotal != 1 {
		t.Fatalf("repeat token counting wrong: raw=%d unique=%d", rawTotal, uniqueTotal)
	}

	longToken := strings.Repeat("x", 257)
	request = httptest.NewRequest(http.MethodGet, "/c/"+campaign.PublicID+"?t="+longToken, nil)
	response = httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), longToken) {
		t.Fatalf("long token handling failed: %d", response.Code)
	}
	var nullHashes int
	if err := application.Database.QueryRow(`SELECT COUNT(*) FROM campaign_visits WHERE campaign_id=? AND install_token_hash IS NULL`, campaign.ID).Scan(&nullHashes); err != nil || nullHashes != 1 {
		t.Fatalf("long token was not ignored: %d %v", nullHashes, err)
	}
}

func TestPublicCampaignUnavailableStatesAreSafe(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, org, campaign := seedActivePublicCampaign(t, application, "camp_states", "states", "en")
	q := db.NewQuerier(application.Database)
	checkUnavailable := func(target string) {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, target, nil)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound || !strings.Contains(response.Body.String(), "currently unavailable") {
			t.Fatalf("%s unsafe unavailable response: %d %s", target, response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), campaign.Name) {
			t.Fatalf("%s leaked campaign details", target)
		}
	}
	checkUnavailable("/c/unknown_campaign")
	for _, status := range []string{"draft", "paused", "archived"} {
		if _, err := application.Database.Exec(`UPDATE campaigns SET status=?,public_link_enabled=1,disabled_at=NULL WHERE id=?`, status, campaign.ID); err != nil {
			t.Fatal(err)
		}
		checkUnavailable("/c/" + campaign.PublicID)
	}
	if _, err := application.Database.Exec(`UPDATE campaigns SET status='active',public_link_enabled=0 WHERE id=?`, campaign.ID); err != nil {
		t.Fatal(err)
	}
	checkUnavailable("/c/" + campaign.PublicID)
	if _, err := application.Database.Exec(`UPDATE campaigns SET public_link_enabled=1,disabled_at=? WHERE id=?`, db.Now(), campaign.ID); err != nil {
		t.Fatal(err)
	}
	checkUnavailable("/c/" + campaign.PublicID)
	if _, err := application.Database.Exec(`UPDATE campaigns SET disabled_at=NULL WHERE id=?`, campaign.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.SetOrganizationDisabled(context.Background(), org.PublicID, true, owner.ID); err != nil {
		t.Fatal(err)
	}
	checkUnavailable("/u/" + org.Slug + "/" + campaign.Slug)
}

func TestPublicVisitQuotaCanBeRaisedAndDashboardShowsCounters(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, org, campaign := seedActivePublicCampaign(t, application, "camp_limit", "limit", "en")
	if _, err := application.Database.Exec(`UPDATE organization_limits SET max_monthly_visits=1 WHERE organization_id=?`, org.ID); err != nil {
		t.Fatal(err)
	}
	first := httptest.NewRecorder()
	application.Handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/c/"+campaign.PublicID, nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first visit failed: %d", first.Code)
	}
	limited := httptest.NewRecorder()
	application.Handler.ServeHTTP(limited, httptest.NewRequest(http.MethodGet, "/c/"+campaign.PublicID, nil))
	lower := strings.ToLower(limited.Body.String())
	if limited.Code != http.StatusServiceUnavailable || !strings.Contains(lower, "safety limit") || strings.Contains(lower, "upgrade") || strings.Contains(lower, "paid") {
		t.Fatalf("quota page invalid: %d %s", limited.Code, limited.Body.String())
	}
	q := db.NewQuerier(application.Database)
	limits, _ := q.GetOrganizationLimits(context.Background(), org.ID)
	limits.MaxMonthlyVisits = 2
	if err := q.UpdateOrganizationLimits(context.Background(), org.ID, limits, owner.ID); err != nil {
		t.Fatal(err)
	}
	again := httptest.NewRecorder()
	application.Handler.ServeHTTP(again, httptest.NewRequest(http.MethodGet, "/c/"+campaign.PublicID, nil))
	if again.Code != http.StatusOK {
		t.Fatalf("visit did not resume after limit increase: %d", again.Code)
	}
	ownerLogin := login(t, application, owner.Username, "a sufficiently long password")
	request := httptest.NewRequest(http.MethodGet, "/app/orgs/"+org.PublicID+"/campaigns/"+campaign.PublicID, nil)
	request.AddCookie(ownerLogin.session)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "Raw visits") || !strings.Contains(response.Body.String(), ">2<") {
		t.Fatalf("dashboard counters missing: %d %s", response.Code, response.Body.String())
	}
}

func TestCampaignReadinessGuidanceRendersInSupportedLocales(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, org, campaign := seedActivePublicCampaign(t, application, "camp_readiness", "readiness", "en")
	ownerLogin := login(t, application, owner.Username, "a sufficiently long password")
	base := "/app/orgs/" + org.PublicID + "/campaigns/" + campaign.PublicID

	for _, tc := range []struct {
		locale, checklist, chrome string
	}{
		{"en", "Campaign setup checklist", "Chrome / Chromium MV3 example"},
		{"de", "Checkliste zur Kampagneneinrichtung", "Beispiel für Chrome / Chromium MV3"},
		{"es", "Lista de configuración de campaña", "Ejemplo para Chrome / Chromium MV3"},
	} {
		request := httptest.NewRequest(http.MethodGet, base+"?lang="+tc.locale, nil)
		request.AddCookie(ownerLogin.session)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		body := response.Body.String()
		if response.Code != http.StatusOK || !strings.Contains(body, tc.checklist) || !strings.Contains(body, tc.chrome) {
			t.Fatalf("campaign readiness locale %s failed: %d %s", tc.locale, response.Code, body)
		}
		if strings.Contains(body, "test-token-123") || !strings.Contains(body, "chrome.runtime.setUninstallURL") {
			t.Fatalf("campaign integration example unsafe or missing for %s", tc.locale)
		}
		if !strings.Contains(body, `data-copy-target="campaign-public-url"`) || !strings.Contains(body, `data-copy-target="chrome-snippet"`) || !strings.Contains(body, `status-active`) {
			t.Fatalf("campaign quality-of-life controls missing for %s", tc.locale)
		}
	}
}

func TestReleaseReadinessDocumentsExistAndAreLinked(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"../../docs/DEPLOYMENT.md",
		"../../docs/BACKUP_RESTORE.md",
		"../../docs/RELEASE_CHECKLIST.md",
		"../../docs/releases/v0.1.3.md",
		"../../ATTRIBUTIONS.md",
	} {
		content, err := os.ReadFile(path)
		if err != nil || len(content) < 200 {
			t.Fatalf("release document %s missing or empty: %v", path, err)
		}
	}
	readme, err := os.ReadFile("../../README.md")
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range []string{"docs/DEPLOYMENT.md", "docs/BACKUP_RESTORE.md", "docs/RELEASE_CHECKLIST.md", "ATTRIBUTIONS.md"} {
		if !strings.Contains(string(readme), link) {
			t.Fatalf("README does not link %s", link)
		}
	}
}

func TestPublicPageLanguageOverrideDoesNotSetCookie(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	_, _, campaign := seedActivePublicCampaign(t, application, "camp_language", "language", "de")
	for _, tc := range []struct {
		query, lang, text string
	}{
		{"", "de", "Freiwilliges Feedback"},
		{"?lang=es", "es", "Comentarios opcionales"},
		{"?lang=fr", "en", "Optional feedback"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/c/"+campaign.PublicID+tc.query, nil)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<html lang="`+tc.lang+`"`) || !strings.Contains(response.Body.String(), tc.text) {
			t.Fatalf("language %q failed: %d %s", tc.query, response.Code, response.Body.String())
		}
		if cookieNamed(response.Result().Cookies(), i18n.LanguageCookie) != nil {
			t.Fatalf("public language override set a cookie")
		}
	}
}

func TestPhase6FormSubmissionPrivacyQuotaAndInboxPermissions(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, org, campaign := seedActivePublicCampaign(t, application, "camp_phase6", "phase6", "en")
	q := db.NewQuerier(application.Database)
	if err := q.CreateFormField(context.Background(), db.SaveFormFieldInput{
		PublicID: "field_feedback", CampaignID: campaign.ID, FieldType: "textarea",
		Label: "What happened?", Required: true, ConfigJSON: `{"max_length":100}`,
	}, owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateFormField(context.Background(), db.SaveFormFieldInput{
		PublicID: "field_rating", CampaignID: campaign.ID, FieldType: "rating_1_5", Label: "Rating",
	}, owner.ID); err != nil {
		t.Fatal(err)
	}

	rawToken := "phase6-raw-token"
	get := httptest.NewRecorder()
	application.Handler.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/c/"+campaign.PublicID+"?t="+rawToken, nil))
	body := get.Body.String()
	if get.Code != http.StatusOK || !strings.Contains(body, `name="field_field_feedback"`) || strings.Contains(body, rawToken) || len(get.Result().Cookies()) != 0 {
		t.Fatalf("public form privacy/rendering failed: %d %s", get.Code, body)
	}
	visitMatch := regexp.MustCompile(`name="visit_public_id" value="([^"]+)"`).FindStringSubmatch(body)
	if len(visitMatch) != 2 {
		t.Fatal("public form did not include a visit public ID")
	}

	submit := func(values url.Values) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/c/"+campaign.PublicID+"/submit?lang=en", strings.NewReader(values.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.Header.Set("User-Agent", "raw-agent-must-not-be-stored")
		application.Handler.ServeHTTP(response, request)
		return response
	}
	invalid := submit(url.Values{"visit_public_id": {visitMatch[1]}, "field_field_rating": {"9"}})
	if invalid.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid submission accepted: %d", invalid.Code)
	}
	honeypot := submit(url.Values{"website": {"spam"}, "field_field_feedback": {"spam"}})
	if honeypot.Code != http.StatusOK {
		t.Fatalf("honeypot did not return generic success: %d", honeypot.Code)
	}
	var count int
	if err := application.Database.QueryRow(`SELECT COUNT(*) FROM campaign_submissions WHERE campaign_id=?`, campaign.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("honeypot stored a submission: %d %v", count, err)
	}
	oversized := submit(url.Values{"field_field_feedback": {strings.Repeat("x", (128<<10)+1)}})
	if oversized.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized submission status=%d", oversized.Code)
	}
	freeText := `<script>alert("private")</script>`
	valid := submit(url.Values{
		"visit_public_id": {visitMatch[1]}, "field_field_feedback": {freeText},
		"field_field_rating": {"5"}, "unknown_field": {"ignored"},
	})
	if valid.Code != http.StatusOK || !strings.Contains(valid.Body.String(), "Thank you") || len(valid.Result().Cookies()) != 0 {
		t.Fatalf("valid submission failed: %d %s", valid.Code, valid.Body.String())
	}
	var visitID any
	var storedHash string
	if err := application.Database.QueryRow(`SELECT visit_id,install_token_hash FROM campaign_submissions WHERE campaign_id=?`, campaign.ID).Scan(&visitID, &storedHash); err != nil || visitID == nil || storedHash == rawToken || storedHash == "" {
		t.Fatalf("submission privacy/linkage failed: visit=%v hash=%q err=%v", visitID, storedHash, err)
	}
	for _, forbiddenColumn := range []string{"ip", "user_agent"} {
		rows, err := application.Database.Query(`SELECT name FROM pragma_table_info('campaign_submissions') WHERE lower(name) LIKE ?`, "%"+forbiddenColumn+"%")
		if err != nil {
			t.Fatal(err)
		}
		if rows.Next() {
			rows.Close()
			t.Fatalf("submission schema contains privacy-forbidden column %q", forbiddenColumn)
		}
		rows.Close()
	}

	analyst := seedNonOwner(t, application, "phase6analyst", "phase6 analyst password")
	viewer := seedNonOwner(t, application, "phase6viewer", "phase6 viewer password")
	for _, member := range []struct {
		user db.User
		role string
	}{{analyst, "analyst"}, {viewer, "viewer"}} {
		if _, err := application.Database.Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id) VALUES(?,?,'member',?,?)`, org.ID, member.user.ID, db.Now(), owner.ID); err != nil {
			t.Fatal(err)
		}
		if err := q.SetCampaignMember(context.Background(), campaign, member.user.PublicID, member.role, owner.ID); err != nil {
			t.Fatal(err)
		}
	}
	base := "/app/orgs/" + org.PublicID + "/campaigns/" + campaign.PublicID
	for _, tc := range []struct {
		user     db.User
		password string
		status   int
	}{{owner, "a sufficiently long password", http.StatusOK}, {analyst, "phase6 analyst password", http.StatusOK}, {viewer, "phase6 viewer password", http.StatusForbidden}} {
		session := login(t, application, tc.user.Username, tc.password)
		request := httptest.NewRequest(http.MethodGet, base+"/responses", nil)
		request.AddCookie(session.session)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("%s inbox status=%d want=%d", tc.user.Username, response.Code, tc.status)
		}
		if tc.status == http.StatusOK && strings.Contains(response.Body.String(), storedHash) {
			t.Fatal("response inbox exposed install token hash")
		}
	}

	ownerLogin := login(t, application, owner.Username, "a sufficiently long password")
	var submissionPublicID string
	if err := application.Database.QueryRow(`SELECT public_id FROM campaign_submissions WHERE campaign_id=?`, campaign.ID).Scan(&submissionPublicID); err != nil {
		t.Fatal(err)
	}
	detailRequest := httptest.NewRequest(http.MethodGet, base+"/responses/"+submissionPublicID, nil)
	detailRequest.AddCookie(ownerLogin.session)
	detail := httptest.NewRecorder()
	application.Handler.ServeHTTP(detail, detailRequest)
	if detail.Code != http.StatusOK || strings.Contains(detail.Body.String(), freeText) || !strings.Contains(detail.Body.String(), "&lt;script&gt;") {
		t.Fatalf("response free text was not escaped: %d %s", detail.Code, detail.Body.String())
	}

	if _, err := application.Database.Exec(`UPDATE organization_limits SET max_monthly_submissions=1 WHERE organization_id=?`, org.ID); err != nil {
		t.Fatal(err)
	}
	limited := submit(url.Values{"field_field_feedback": {"second"}})
	lower := strings.ToLower(limited.Body.String())
	if limited.Code != http.StatusServiceUnavailable || !strings.Contains(lower, "safety limit") || strings.Contains(lower, "paid") || strings.Contains(lower, "upgrade") {
		t.Fatalf("submission quota response invalid: %d %s", limited.Code, limited.Body.String())
	}
	if _, err := application.Database.Exec(`UPDATE organization_limits SET max_monthly_submissions=2 WHERE organization_id=?`, org.ID); err != nil {
		t.Fatal(err)
	}
	if raised := submit(url.Values{"field_field_feedback": {"second"}}); raised.Code != http.StatusOK {
		t.Fatalf("raised submission quota did not allow submission: %d", raised.Code)
	}

	privateOwner := seedNonOwner(t, application, "privateowner", "private owner password")
	privateOrg, err := q.CreateOrganization(context.Background(), db.CreateOrganizationInput{
		PublicID: "org_private", Slug: "private", Name: "Private", UserID: privateOwner.ID,
		Limits: db.DefaultLimits{
			MaxOrganizationsPerUser: 2, MaxCampaignsPerOrg: 3, MaxMembersPerOrg: 5,
			MaxActiveInvitesPerOrg: 10, MaxMonthlyVisitsPerOrg: 100, MaxMonthlySubmissionsPerOrg: 100,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	privateCampaign, err := q.CreateCampaign(context.Background(), db.CreateCampaignInput{
		PublicID: "camp_private_responses", OrganizationID: privateOrg.ID, CreatedBy: privateOwner.ID,
		Name: "Private responses", Slug: "private-responses", Language: "en", PrivacyPreset: "strict",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.CreateSubmission(context.Background(), db.CreateSubmissionInput{
		PublicID: "submission_private", CampaignID: privateCampaign.ID, OrgID: privateOrg.ID, SubmittedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	instanceOwnerRequest := httptest.NewRequest(http.MethodGet, "/app/orgs/"+privateOrg.PublicID+"/campaigns/"+privateCampaign.PublicID+"/responses", nil)
	instanceOwnerRequest.AddCookie(ownerLogin.session)
	instanceOwnerResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(instanceOwnerResponse, instanceOwnerRequest)
	if instanceOwnerResponse.Code != http.StatusForbidden {
		t.Fatalf("instance owner browsed private responses without membership: %d", instanceOwnerResponse.Code)
	}
}

func TestPhase6FormBuilderPermissionsAndArchivedReadOnly(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, password := seedOwner(t, application)
	q := db.NewQuerier(application.Database)
	orgs, _ := q.ListOrganizationsForUser(context.Background(), owner.ID)
	org := orgs[0]
	campaign, err := q.CreateCampaign(context.Background(), db.CreateCampaignInput{
		PublicID: "camp_builder", OrganizationID: org.ID, CreatedBy: owner.ID,
		Name: "Builder", Slug: "builder", Language: "en", PrivacyPreset: "strict",
	})
	if err != nil {
		t.Fatal(err)
	}
	campaign, _ = q.GetCampaignByPublicID(context.Background(), org.PublicID, campaign.PublicID)
	base := "/app/orgs/" + org.PublicID + "/campaigns/" + campaign.PublicID + "/form"

	users := []struct {
		name, password, role string
		canEdit              bool
	}{
		{"formeditor", "form editor password", "editor", true},
		{"formanalyst", "form analyst password", "analyst", false},
		{"formviewer", "form viewer password", "viewer", false},
	}
	for _, tc := range users {
		user := seedNonOwner(t, application, tc.name, tc.password)
		if _, err := application.Database.Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id) VALUES(?,?,'member',?,?)`, org.ID, user.ID, db.Now(), owner.ID); err != nil {
			t.Fatal(err)
		}
		if err := q.SetCampaignMember(context.Background(), campaign, user.PublicID, tc.role, owner.ID); err != nil {
			t.Fatal(err)
		}
		session := login(t, application, user.Username, tc.password)
		csrfCookie, token, _ := csrfPage(t, application, base, session.session)
		response := formPost(application, base+"/fields", url.Values{
			"csrf_token": {token}, "field_type": {"textarea"}, "label": {"From " + tc.role}, "max_length": {"1000"},
		}, session.session, csrfCookie)
		expected := http.StatusForbidden
		if tc.canEdit {
			expected = http.StatusSeeOther
		}
		if response.Code != expected {
			t.Fatalf("%s add field=%d want=%d", tc.role, response.Code, expected)
		}
	}
	outsider := seedNonOwner(t, application, "formoutsider", "form outsider password")
	outsiderLogin := login(t, application, outsider.Username, "form outsider password")
	request := httptest.NewRequest(http.MethodGet, base, nil)
	request.AddCookie(outsiderLogin.session)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("non-member form access=%d", response.Code)
	}

	ownerLogin := login(t, application, owner.Username, password)
	csrfCookie, token, _ := csrfPage(t, application, base, ownerLogin.session)
	ownerCreated := formPost(application, base+"/fields", url.Values{
		"csrf_token": {token}, "field_type": {"rating_1_5"}, "label": {"Owner rating"},
	}, ownerLogin.session, csrfCookie)
	if ownerCreated.Code != http.StatusSeeOther {
		t.Fatalf("owner add field=%d", ownerCreated.Code)
	}
	if _, err := application.Database.Exec(`UPDATE campaigns SET status='archived',archived_at=? WHERE id=?`, db.Now(), campaign.ID); err != nil {
		t.Fatal(err)
	}
	_, _, archivedBody := csrfPage(t, application, base, ownerLogin.session, csrfCookie)
	if !strings.Contains(archivedBody, "read-only") || strings.Contains(archivedBody, `action="`+base+`/fields"`) {
		t.Fatalf("archived form was not read-only: %s", archivedBody)
	}
}

func TestPhase7AnalyticsExportsRetentionAndPermissions(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	owner, org, campaign := seedActivePublicCampaign(t, application, "camp_phase7", "phase7", "en")
	q := db.NewQuerier(application.Database)
	ctx := context.Background()
	if err := q.CreateFormField(ctx, db.SaveFormFieldInput{PublicID: "field_phase7", CampaignID: campaign.ID, FieldType: "textarea", Label: `Why, exactly?`}, owner.ID); err != nil {
		t.Fatal(err)
	}
	field, _ := q.GetFormField(ctx, campaign.ID, "field_phase7")
	now := time.Now().UTC()
	if err := q.RecordCampaignVisit(ctx, db.RecordVisitInput{
		PublicID: "visit_phase7", CampaignID: campaign.ID, OrganizationID: org.ID, TokenHash: "secret-hash-not-exported",
		ReferrerDomain: "example.org", CoarseBrowser: "Firefox", CoarseOS: "Linux",
		URLContext: map[string]string{"platform": "firefox", "utm_campaign": "uninstall"},
		CountRaw:   true, CountUnique: true, CollectToken: true, CreatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal("line one,\nline two")
	if err := q.CreateSubmission(ctx, db.CreateSubmissionInput{
		PublicID: "submission_phase7", VisitPublicID: "visit_phase7", CampaignID: campaign.ID, OrgID: org.ID, SubmittedAt: now,
		Answers: []db.SubmissionAnswerInput{{FieldID: field.ID, FieldPublicID: field.PublicID, FieldType: field.FieldType, FieldLabelSnapshot: field.Label, ValueJSON: string(raw)}},
	}); err != nil {
		t.Fatal(err)
	}
	analyst := seedNonOwner(t, application, "phase7analyst", "phase7 analyst password")
	viewer := seedNonOwner(t, application, "phase7viewer", "phase7 viewer password")
	for _, member := range []struct {
		user db.User
		role string
	}{{analyst, "analyst"}, {viewer, "viewer"}} {
		if _, err := application.Database.Exec(`INSERT INTO organization_members(organization_id,user_id,role,created_at,created_by_user_id) VALUES(?,?,'member',?,?)`, org.ID, member.user.ID, db.Now(), owner.ID); err != nil {
			t.Fatal(err)
		}
		if err := q.SetCampaignMember(ctx, campaign, member.user.PublicID, member.role, owner.ID); err != nil {
			t.Fatal(err)
		}
	}
	base := "/app/orgs/" + org.PublicID + "/campaigns/" + campaign.PublicID
	for _, tc := range []struct {
		user     db.User
		password string
		status   int
	}{{owner, "a sufficiently long password", http.StatusOK}, {analyst, "phase7 analyst password", http.StatusOK}, {viewer, "phase7 viewer password", http.StatusForbidden}} {
		session := login(t, application, tc.user.Username, tc.password)
		request := httptest.NewRequest(http.MethodGet, base+"/analytics?range=7", nil)
		request.AddCookie(session.session)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("%s analytics status=%d want=%d", tc.user.Username, response.Code, tc.status)
		}
		if tc.status == http.StatusOK {
			body := response.Body.String()
			if !strings.Contains(body, "Submission rate") || !strings.Contains(body, "100.0%") || strings.Contains(body, "example.org") || strings.Contains(body, "raw-agent") || strings.Contains(body, "https://cdn") {
				t.Fatalf("analytics overview/privacy incorrect: %s", body)
			}
		}
	}

	ownerLogin := login(t, application, owner.Username, "a sufficiently long password")
	for _, locale := range []struct {
		code, heading string
	}{{"en", "Analytics"}, {"de", "Analysen"}, {"es", "Analítica"}} {
		request := httptest.NewRequest(http.MethodGet, base+"/analytics?lang="+locale.code, nil)
		request.AddCookie(ownerLogin.session)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "<h1>"+locale.heading+"</h1>") {
			t.Fatalf("analytics locale %s failed: %d", locale.code, response.Code)
		}
	}
	csrfCookie, token, _ := csrfPage(t, application, base+"/privacy", ownerLogin.session)
	invalidRetention := formPost(application, base+"/privacy", url.Values{
		"csrf_token": {token}, "public_language_default": {"en"}, "retention_enabled": {"on"}, "retention_days": {"31"},
	}, ownerLogin.session, csrfCookie)
	if invalidRetention.Code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid retention accepted: %d", invalidRetention.Code)
	}
	validRetention := formPost(application, base+"/privacy", url.Values{
		"csrf_token": {token}, "public_language_default": {"en"}, "retention_enabled": {"on"}, "retention_days": {"90"},
		"collect_referrer_domain": {"on"}, "collect_coarse_browser": {"on"}, "collect_coarse_os": {"on"},
	}, ownerLogin.session, csrfCookie)
	if validRetention.Code != http.StatusSeeOther {
		t.Fatalf("valid retention rejected: %d", validRetention.Code)
	}
	analyticsRequest := httptest.NewRequest(http.MethodGet, base+"/analytics?lang=de", nil)
	analyticsRequest.AddCookie(ownerLogin.session)
	analyticsResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(analyticsResponse, analyticsRequest)
	if analyticsResponse.Code != http.StatusOK || !strings.Contains(analyticsResponse.Body.String(), "Referrer-Domains") || !strings.Contains(analyticsResponse.Body.String(), "älter als 90 Tage") {
		t.Fatalf("enabled metadata/translation missing: %d %s", analyticsResponse.Code, analyticsResponse.Body.String())
	}

	for _, tc := range []struct {
		path, contentType string
	}{
		{"/export/submissions.csv", "text/csv"},
		{"/export/submissions.json", "application/json"},
	} {
		request := httptest.NewRequest(http.MethodGet, base+tc.path, nil)
		request.AddCookie(ownerLogin.session)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		body := response.Body.String()
		if response.Code != http.StatusOK || !strings.Contains(response.Header().Get("Content-Type"), tc.contentType) || !strings.Contains(response.Header().Get("Content-Disposition"), "phase7-submissions") {
			t.Fatalf("%s export headers failed: %d %#v", tc.path, response.Code, response.Header())
		}
		if !strings.Contains(body, "submission_phase7") || !strings.Contains(body, "line one") || !strings.Contains(body, "firefox") || !strings.Contains(body, "uninstall") || strings.Contains(body, "secret-hash-not-exported") || strings.Contains(body, "raw-agent") || strings.Contains(body, `"campaign_id"`) {
			t.Fatalf("%s export content unsafe: %s", tc.path, body)
		}
	}
	var exportAudits int
	if err := application.Database.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action IN ('campaign.export.csv','campaign.export.json') AND target_id=?`, campaign.PublicID).Scan(&exportAudits); err != nil || exportAudits != 2 {
		t.Fatalf("exports not audited: %d %v", exportAudits, err)
	}
	viewerLogin := login(t, application, viewer.Username, "phase7 viewer password")
	request := httptest.NewRequest(http.MethodGet, base+"/export/submissions.csv", nil)
	request.AddCookie(viewerLogin.session)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden {
		t.Fatalf("viewer export status=%d", response.Code)
	}

	noCSRF := formPost(application, base+"/responses/delete-all", url.Values{"confirmation": {campaign.Slug}}, ownerLogin.session)
	if noCSRF.Code != http.StatusForbidden {
		t.Fatalf("delete without CSRF=%d", noCSRF.Code)
	}
	wrong := formPost(application, base+"/responses/delete-all", url.Values{"csrf_token": {token}, "confirmation": {"wrong"}}, ownerLogin.session, csrfCookie)
	if wrong.Code != http.StatusUnprocessableEntity {
		t.Fatalf("wrong deletion confirmation=%d", wrong.Code)
	}
	analystLogin := login(t, application, analyst.Username, "phase7 analyst password")
	analystDelete := formPost(application, base+"/visits/delete-all", url.Values{"csrf_token": {token}, "confirmation": {campaign.Slug}}, analystLogin.session, csrfCookie)
	if analystDelete.Code != http.StatusForbidden {
		t.Fatalf("analyst deletion=%d", analystDelete.Code)
	}
	deleteVisits := formPost(application, base+"/visits/delete-all", url.Values{"csrf_token": {token}, "confirmation": {campaign.Slug}}, ownerLogin.session, csrfCookie)
	if deleteVisits.Code != http.StatusSeeOther {
		t.Fatalf("visit deletion failed: %d", deleteVisits.Code)
	}
	var linked any
	if err := application.Database.QueryRow(`SELECT visit_id FROM campaign_submissions WHERE public_id='submission_phase7'`).Scan(&linked); err != nil || linked != nil {
		t.Fatalf("submission not preserved/unlinked: %v %v", linked, err)
	}
	deleteResponses := formPost(application, base+"/responses/delete-all", url.Values{"csrf_token": {token}, "confirmation": {campaign.Slug}}, ownerLogin.session, csrfCookie)
	if deleteResponses.Code != http.StatusSeeOther {
		t.Fatalf("response deletion failed: %d", deleteResponses.Code)
	}

	privateOwner := seedNonOwner(t, application, "phase7private", "phase7 private password")
	privateOrg, err := q.CreateOrganization(ctx, db.CreateOrganizationInput{
		PublicID: "org_phase7_private", Slug: "phase7-private", Name: "Private analytics", UserID: privateOwner.ID,
		Limits: db.DefaultLimits{MaxOrganizationsPerUser: 2, MaxCampaignsPerOrg: 3, MaxMembersPerOrg: 5, MaxActiveInvitesPerOrg: 10, MaxMonthlyVisitsPerOrg: 100, MaxMonthlySubmissionsPerOrg: 100},
	})
	if err != nil {
		t.Fatal(err)
	}
	privateCampaign, err := q.CreateCampaign(ctx, db.CreateCampaignInput{PublicID: "camp_phase7_private", OrganizationID: privateOrg.ID, CreatedBy: privateOwner.ID, Name: "Private", Slug: "private", Language: "en", PrivacyPreset: "strict"})
	if err != nil {
		t.Fatal(err)
	}
	privateRequest := httptest.NewRequest(http.MethodGet, "/app/orgs/"+privateOrg.PublicID+"/campaigns/"+privateCampaign.PublicID+"/analytics", nil)
	privateRequest.AddCookie(ownerLogin.session)
	privateResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(privateResponse, privateRequest)
	if privateResponse.Code != http.StatusForbidden {
		t.Fatalf("instance owner viewed private analytics: %d", privateResponse.Code)
	}
}
func TestI18nPlaceholdersAndForbiddenStrings(t *testing.T) {
	application := testApp(t)

	routes := []string{
		"/",
		"/login",
		"/register",
		"/legal/privacy",
		"/legal/imprint",
	}

	for _, route := range routes {
		for _, lang := range []string{"en", "de", "es"} {
			req := httptest.NewRequest("GET", route, nil)
			req.AddCookie(&http.Cookie{Name: "lang", Value: lang})
			rr := httptest.NewRecorder()
			application.Handler.ServeHTTP(rr, req)

			body := rr.Body.String()
			if strings.Contains(body, "[missing:") {
				t.Errorf("Route %s (%s) contains missing i18n key", route, lang)
			}
			lowerBody := strings.ToLower(body)
			if strings.Contains(lowerBody, "ohne ihnen zu folgen") || strings.Contains(lowerBody, "following them around") {
				t.Errorf("Route %s (%s) contains forbidden wording", route, lang)
			}
		}
	}
}
