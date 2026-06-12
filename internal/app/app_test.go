package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
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
		if !strings.Contains(response.Body.String(), `<html lang="`+test.lang+`">`) {
			t.Fatalf("%s did not render lang=%s", test.target, test.lang)
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
	if !strings.Contains(body, `<html lang="en">`) || !strings.Contains(body, "English is shown as the fallback") {
		t.Fatalf("legal page did not clearly fall back to English: %s", body)
	}
}

func TestSecurityHeadersAssetsAndNoExternalCDN(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	request := httptest.NewRequest(http.MethodGet, "/setup", nil)
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	for header, want := range map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "no-referrer",
		"X-Frame-Options":        "DENY",
	} {
		if got := response.Header().Get(header); got != want {
			t.Fatalf("%s: expected %q, got %q", header, want, got)
		}
	}
	if !strings.Contains(response.Header().Get("Content-Security-Policy"), "default-src 'self'") {
		t.Fatal("missing restrictive CSP")
	}
	if strings.Contains(response.Body.String(), "https://") || strings.Contains(response.Body.String(), "http://") {
		t.Fatal("rendered HTML contains an external URL")
	}

	assetRequest := httptest.NewRequest(http.MethodGet, "/assets/app.css", nil)
	assetResponse := httptest.NewRecorder()
	application.Handler.ServeHTTP(assetResponse, assetRequest)
	if assetResponse.Code != http.StatusOK || !strings.Contains(assetResponse.Body.String(), ":root") {
		t.Fatalf("local CSS asset was not served")
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
		if strings.Contains(response.Body.String(), "https://") || strings.Contains(response.Body.String(), "http://") {
			t.Fatalf("%s contains external link", tc.path)
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
	if !strings.Contains(body, "Moderación de campañas") || strings.Contains(body, "https://cdn") {
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
	if _, err := application.Database.Exec(`UPDATE campaign_settings SET collect_referrer_domain=1,collect_coarse_browser=1,collect_coarse_os=1 WHERE campaign_id=?`, campaign.ID); err != nil {
		t.Fatal(err)
	}
	rawToken := "opaque-install-token"
	request := httptest.NewRequest(http.MethodGet, "/c/"+campaign.PublicID+"?t="+rawToken, nil)
	request.Header.Set("Referer", "https://Example.COM/private/path?secret=value")
	request.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0) AppleWebKit Chrome/124.0 Safari/537.36")
	response := httptest.NewRecorder()
	application.Handler.ServeHTTP(response, request)
	body := response.Body.String()
	if response.Code != http.StatusOK || !strings.Contains(body, `<html lang="es">`) || !strings.Contains(body, "Sentimos que te vayas") {
		t.Fatalf("public campaign by ID failed: status=%d body=%s", response.Code, body)
	}
	if len(response.Result().Cookies()) != 0 {
		t.Fatalf("public page set cookies: %#v", response.Result().Cookies())
	}
	if strings.Contains(body, rawToken) || strings.Contains(body, "https://cdn") || strings.Contains(body, "<script") {
		t.Fatal("public page leaked token or loaded scripts/external assets")
	}
	mac := hmac.New(sha256.New, []byte(application.Config.Secret))
	_, _ = mac.Write([]byte(rawToken))
	expectedHash := hex.EncodeToString(mac.Sum(nil))
	var storedHash, referrer, browser, os string
	if err := application.Database.QueryRow(`SELECT install_token_hash,referrer_domain,coarse_browser,coarse_os FROM campaign_visits ORDER BY id LIMIT 1`).Scan(&storedHash, &referrer, &browser, &os); err != nil {
		t.Fatal(err)
	}
	if storedHash != expectedHash || storedHash == rawToken {
		t.Fatalf("install token was not HMAC-hashed: %q", storedHash)
	}
	if referrer != "example.com" || browser != "Chrome" || os != "Windows" {
		t.Fatalf("coarse metadata incorrect: referrer=%q browser=%q os=%q", referrer, browser, os)
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
	if rawTotal != 3 || uniqueTotal != 1 {
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

func TestPublicPageLanguageOverrideDoesNotSetCookie(t *testing.T) {
	t.Parallel()
	application := testApp(t)
	_, _, campaign := seedActivePublicCampaign(t, application, "camp_language", "language", "de")
	for _, tc := range []struct {
		query, lang, text string
	}{
		{"", "de", "Schade, dass du gehst"},
		{"?lang=es", "es", "Sentimos que te vayas"},
		{"?lang=fr", "en", "Sorry to see you go"},
	} {
		request := httptest.NewRequest(http.MethodGet, "/c/"+campaign.PublicID+tc.query, nil)
		response := httptest.NewRecorder()
		application.Handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `<html lang="`+tc.lang+`">`) || !strings.Contains(response.Body.String(), tc.text) {
			t.Fatalf("language %q failed: %d %s", tc.query, response.Code, response.Body.String())
		}
		if cookieNamed(response.Result().Cookies(), i18n.LanguageCookie) != nil {
			t.Fatalf("public language override set a cookie")
		}
	}
}
