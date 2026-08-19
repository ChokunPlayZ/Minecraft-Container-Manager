package api

import (
	"net/http"

	"github.com/mcm-panel/mcm/internal/auth"
)

type totpEnrollRequest struct{}

type totpConfirmRequest struct {
	Code string `json:"code"`
}

type totpDisableRequest struct {
	Code string `json:"code"`
}

// handleTOTPEnroll begins TOTP enrollment: it generates a secret, persists it
// (still disabled), and returns the QR URI so the user can scan it.
func (s *Server) handleTOTPEnroll(w http.ResponseWriter, r *http.Request) {
	userID := s.currentUserID(r)
	user, err := s.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "session invalid")
		return
	}
	if user.TOTPEnabled {
		writeError(w, http.StatusConflict, "totp_enabled", "totp is already enabled")
		return
	}

	secret, err := auth.GenerateTOTPSecret()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not generate totp secret")
		return
	}
	if err := s.users.SetTOTPSecret(r.Context(), userID, secret); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not persist totp secret")
		return
	}
	uri := auth.TOTPURI("MCM", user.Email, secret)
	writeJSON(w, http.StatusOK, map[string]string{
		"secret": secret,
		"qr_uri": uri,
	})
}

// handleTOTPConfirm verifies a code against the pending secret and enables TOTP.
func (s *Server) handleTOTPConfirm(w http.ResponseWriter, r *http.Request) {
	userID := s.currentUserID(r)
	user, err := s.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "session invalid")
		return
	}
	if user.TOTPEnabled {
		writeError(w, http.StatusConflict, "totp_enabled", "totp is already enabled")
		return
	}
	if user.TOTPSecret == "" {
		writeError(w, http.StatusBadRequest, "not_enrolled", "start enrollment first")
		return
	}

	var req totpConfirmRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if !auth.VerifyTOTP(user.TOTPSecret, req.Code, 1) {
		writeError(w, http.StatusBadRequest, "invalid_code", "invalid totp code")
		return
	}
	if err := s.users.EnableTOTP(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not enable totp")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"totp_enabled": true})
}

// handleTOTPDisable verifies the current code and disables TOTP.
func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
	userID := s.currentUserID(r)
	user, err := s.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "session invalid")
		return
	}
	if !user.TOTPEnabled {
		writeError(w, http.StatusBadRequest, "not_enabled", "totp is not enabled")
		return
	}

	var req totpDisableRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if !auth.VerifyTOTP(user.TOTPSecret, req.Code, 1) {
		writeError(w, http.StatusBadRequest, "invalid_code", "invalid totp code")
		return
	}
	if err := s.users.DisableTOTP(r.Context(), userID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not disable totp")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"totp_enabled": false})
}

// handleTOTPStatus reports whether TOTP is currently enabled.
func (s *Server) handleTOTPStatus(w http.ResponseWriter, r *http.Request) {
	userID := s.currentUserID(r)
	user, err := s.users.GetByID(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "unauthorized", "session invalid")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"totp_enabled": user.TOTPEnabled})
}
