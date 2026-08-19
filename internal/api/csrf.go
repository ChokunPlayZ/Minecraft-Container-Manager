package api

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

// CSRFCookieName is the cookie used for the double-submit CSRF token.
const CSRFCookieName = "mcm_csrf"

// csrfHeaderName is the header carrying the CSRF token on state-changing
// requests.
const csrfHeaderName = "X-CSRF-Token"

// csrfSafeMethods are HTTP methods that do not require a CSRF token.
var csrfSafeMethods = map[string]bool{
	http.MethodGet:     true,
	http.MethodHead:    true,
	http.MethodOptions: true,
}

// generateCSRFToken returns a random 32-byte hex token.
func generateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// csrfExemptPaths are unauthenticated or bootstrap endpoints allowed to skip
// CSRF validation (boarding has no session and login stands alone).
var csrfExemptPaths = map[string]bool{
	"/api/onboarding":           true,
	"/api/auth/login":           true,
	"/api/auth/csrf":            true,
	"/api/passkey/login/begin":  true,
	"/api/passkey/login/finish": true,
}

// handleCSRF issues a fresh double-submit CSRF token. It sets a non-HttpOnly
// cookie (so the SPA can read it) and returns the token in the body.
func (s *Server) handleCSRF(w http.ResponseWriter, r *http.Request) {
	token, err := generateCSRFToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not generate csrf token")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
	})
	writeJSON(w, http.StatusOK, map[string]string{"csrf_token": token})
}

// csrfTokenMatches compares two tokens in constant time.
func csrfTokenMatches(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}
