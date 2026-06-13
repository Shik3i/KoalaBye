package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFlashLifecycleAndSignature(t *testing.T) {
	const secret = "test-secret"
	writer := httptest.NewRecorder()
	SetFlash(writer, secret, false, "success", "toast.saved")
	cookies := writer.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	var seen Flash
	handler := FlashMiddleware(secret, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		seen, ok = FlashFromContext(r.Context())
		if !ok {
			t.Fatal("flash missing from context")
		}
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(cookies[0])
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if seen.Key != "toast.saved" || seen.Kind != "success" {
		t.Fatalf("flash=%#v", seen)
	}
	if cleared := response.Result().Cookies(); len(cleared) != 1 || cleared[0].MaxAge != -1 {
		t.Fatalf("flash cookie was not cleared: %#v", cleared)
	}

	request = httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: flashCookieName, Value: cookies[0].Value + "tampered"})
	response = httptest.NewRecorder()
	called := false
	FlashMiddleware(secret, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if _, ok := FlashFromContext(r.Context()); ok {
			t.Fatal("tampered flash accepted")
		}
	})).ServeHTTP(response, request)
	if !called {
		t.Fatal("middleware did not continue")
	}
}
