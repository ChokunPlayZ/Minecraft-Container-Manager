package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateCSRFToken(t *testing.T) {
	a, err := generateCSRFToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	b, err := generateCSRFToken()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if a == b {
		t.Fatal("expected distinct tokens")
	}
	if len(a) != 64 {
		t.Fatalf("expected 64 hex chars, got %d", len(a))
	}
}

func TestCSRFTokenMatches(t *testing.T) {
	if !csrfTokenMatches("abc", "abc") {
		t.Fatal("expected equal tokens to match")
	}
	if csrfTokenMatches("abc", "abd") {
		t.Fatal("expected different tokens not to match")
	}
	if csrfTokenMatches("", "abc") || csrfTokenMatches("abc", "") {
		t.Fatal("expected empty tokens not to match")
	}
}

func TestCSRFExemptAndValidate(t *testing.T) {
	// Safe methods are exempt even with no token.
	r := httptest.NewRequest(http.MethodGet, "/api/servers", nil)
	if !csrfExempt(r) {
		t.Fatal("expected GET to be CSRF-exempt")
	}

	// Onboarding and login are exempt regardless of method.
	for _, path := range []string{"/api/onboarding", "/api/auth/login", "/api/passkey/login/begin", "/api/passkey/login/finish"} {
		r := httptest.NewRequest(http.MethodPost, path, nil)
		if !csrfExempt(r) {
			t.Fatalf("expected %s to be CSRF-exempt", path)
		}
	}

	// A state-changing request with a matching cookie+header validates.
	token, _ := generateCSRFToken()
	r = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: token})
	r.Header.Set(csrfHeaderName, token)
	if !csrfTokenValid(r) {
		t.Fatal("expected valid CSRF to pass")
	}

	// Mismatch fails.
	r = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: token})
	r.Header.Set(csrfHeaderName, "wrong")
	if csrfTokenValid(r) {
		t.Fatal("expected mismatched CSRF to fail")
	}

	// Missing cookie fails.
	r = httptest.NewRequest(http.MethodPost, "/api/auth/logout", nil)
	r.Header.Set(csrfHeaderName, token)
	if csrfTokenValid(r) {
		t.Fatal("expected missing cookie CSRF to fail")
	}
}
