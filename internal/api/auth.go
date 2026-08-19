package api

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/mcm-panel/mcm/internal/auth"
)

type onboardingRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// handleOnboardingStatus reports whether the instance still needs its first
// admin account. The frontend checks this to route a fresh install to the
// onboarding screen instead of dead-ending on login.
func (s *Server) handleOnboardingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	count, err := s.users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not check users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"onboarding_required": count == 0})
}

func (s *Server) handleOnboarding(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	count, err := s.users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not check users")
		return
	}
	if count > 0 {
		writeError(w, http.StatusConflict, "onboarding_complete", "a user already exists")
		return
	}

	var req onboardingRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.Email == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "email and password are required")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not hash password")
		return
	}
	user, err := s.users.Create(r.Context(), req.Email, hash)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create user")
		return
	}
	s.issueSession(w, r, user.ID)
	writeJSON(w, http.StatusCreated, user)
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTPCode string `json:"totp_code"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	ipKey := s.clientIP(r)
	now := time.Now()
	if ok, retryAfter := s.loginLimit.allow(ipKey, now); !ok {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(retryAfter.Seconds())))
		writeError(w, http.StatusTooManyRequests, "rate_limited", "too many login attempts, try again later")
		return
	}

	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	user, err := s.users.GetByEmail(r.Context(), req.Email)
	if errors.Is(err, sql.ErrNoRows) {
		s.loginLimit.record(ipKey, true, now)
		s.loginLimit.record(req.Email, true, now)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not look up user")
		return
	}
	if err := auth.VerifyPassword(req.Password, user.PasswordHash); err != nil {
		s.loginLimit.record(ipKey, true, now)
		s.loginLimit.record(req.Email, true, now)
		writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	if user.TOTPEnabled {
		if req.TOTPCode == "" {
			writeJSON(w, http.StatusAccepted, map[string]bool{"2fa_required": true})
			return
		}
		if !auth.VerifyTOTP(user.TOTPSecret, req.TOTPCode, 1) {
			s.loginLimit.record(ipKey, true, now)
			s.loginLimit.record(req.Email, true, now)
			writeError(w, http.StatusUnauthorized, "invalid_credentials", "invalid or missing totp code")
			return
		}
	}
	s.loginLimit.record(ipKey, false, now)
	s.loginLimit.record(req.Email, false, now)
	s.issueSession(w, r, user.ID)
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie(auth.CookieName)
	if err == nil {
		_ = s.sessions.Delete(r.Context(), cookie.Value)
	}
	clearSessionCookie(w)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	userID := s.currentUserID(r)
	user, err := s.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "session invalid")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) issueSession(w http.ResponseWriter, r *http.Request, userID string) {
	token, err := s.sessions.Create(r.Context(), userID)
	if err != nil {
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(auth.SessionLifetime / time.Second),
	})
}

func (s *Server) secureCookies() bool {
	if s.cfg == nil {
		return false
	}
	return s.cfg.SecureCookies
}

func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     auth.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
