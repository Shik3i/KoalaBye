package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/koalastuff/koalabye/internal/config"
	"github.com/koalastuff/koalabye/internal/db"
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
