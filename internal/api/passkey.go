package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/mcm-panel/mcm/internal/auth"
)

// passkeyFinishEnvelope captures the extra fields the client sends alongside
// the raw WebAuthn credential/assertion JSON. Unknown credential fields in the
// same body are tolerated by the protocol parsers.
type passkeyFinishEnvelope struct {
	RegistrationID string `json:"registration_id"`
	LoginID        string `json:"login_id"`
	Name           string `json:"name"`
}

func jsonUnmarshal(data []byte, v any) error {
	return json.Unmarshal(data, v)
}

// buildWAUser loads a user and their passkeys as a webauthn.User.
func (s *Server) buildWAUser(r *http.Request, userID string) (*auth.WAUser, error) {
	user, err := s.users.GetByID(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	creds, err := s.passkeys.ListByUser(r.Context(), userID)
	if err != nil {
		return nil, err
	}
	return auth.NewWAUser(user, creds), nil
}

// handlePasskeyRegisterBegin starts a passkey registration ceremony for the
// authenticated user.
func (s *Server) handlePasskeyRegisterBegin(w http.ResponseWriter, r *http.Request) {
	userID := s.currentUserID(r)
	wu, err := s.buildWAUser(r, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load user")
		return
	}
	opts := []webauthn.RegistrationOption{
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			UserVerification: protocol.VerificationRequired,
		}),
	}
	if creds := wu.WebAuthnCredentials(); len(creds) > 0 {
		opts = append(opts, webauthn.WithExclusions(webauthn.Credentials(creds).CredentialDescriptors()))
	}

	creation, session, err := s.webAuthn.BeginRegistration(wu, opts...)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not begin registration")
		return
	}
	id := s.ceremonies.save(session)
	writeJSON(w, http.StatusOK, map[string]any{
		"registration_id": id,
		"options":         creation,
	})
}

// handlePasskeyRegisterFinish completes a passkey registration ceremony.
func (s *Server) handlePasskeyRegisterFinish(w http.ResponseWriter, r *http.Request) {
	userID := s.currentUserID(r)
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "could not read body")
		return
	}

	var env passkeyFinishEnvelope
	_ = jsonUnmarshal(body, &env)

	session, ok := s.ceremonies.take(env.RegistrationID)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_session", "registration session not found or expired")
		return
	}
	if !bytes.Equal([]byte(userID), session.UserID) {
		writeError(w, http.StatusBadRequest, "invalid_session", "registration session does not match user")
		return
	}
	parsed, err := protocol.ParseCredentialCreationResponseBytes(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_credential", "invalid registration response")
		return
	}

	wu, err := s.buildWAUser(r, userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load user")
		return
	}
	credential, err := s.webAuthn.CreateCredential(wu, session, parsed)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_credential", "could not validate registration")
		return
	}
	name := env.Name
	if name == "" {
		name = "Passkey"
	}
	if err := s.passkeys.Add(r.Context(), userID, name, credential); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not store passkey")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

// handlePasskeyLoginBegin starts a discoverable (usernameless) passkey login.
func (s *Server) handlePasskeyLoginBegin(w http.ResponseWriter, r *http.Request) {
	assertion, session, err := s.webAuthn.BeginDiscoverableMediatedLogin(protocol.MediationConditional)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not begin login")
		return
	}
	id := s.ceremonies.save(session)
	writeJSON(w, http.StatusOK, map[string]any{
		"login_id": id,
		"options":  assertion,
	})
}

// handlePasskeyLoginFinish completes a passkey assertion and issues a session.
func (s *Server) handlePasskeyLoginFinish(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "could not read body")
		return
	}
	var env passkeyFinishEnvelope
	_ = jsonUnmarshal(body, &env)

	session, ok := s.ceremonies.take(env.LoginID)
	if !ok {
		writeError(w, http.StatusBadRequest, "invalid_session", "login session not found or expired")
		return
	}
	parsed, err := protocol.ParseCredentialRequestResponseBytes(body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_assertion", "invalid login response")
		return
	}

	wuser, credential, err := s.webAuthn.ValidatePasskeyLogin(s.discoverableUserHandler, session, parsed)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid_assertion", "passkey verification failed")
		return
	}
	wu, ok := wuser.(*auth.WAUser)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "unexpected user type")
		return
	}
	if err := s.passkeys.Update(r.Context(), wu.UserID(), credential); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not update passkey")
		return
	}
	s.issueSession(w, r, wu.UserID())
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// discoverableUserHandler resolves a user from a passkey credential ID. It has
// no HTTP request, so it uses the background context for database access.
func (s *Server) discoverableUserHandler(rawID, userHandle []byte) (webauthn.User, error) {
	ctx := context.Background()
	userID, _, err := s.passkeys.GetByCredentialID(ctx, rawID)
	if err != nil {
		return nil, err
	}
	user, err := s.users.GetByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	creds, err := s.passkeys.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return auth.NewWAUser(user, creds), nil
}

// handlePasskeyList returns a user's passkeys.
func (s *Server) handlePasskeyList(w http.ResponseWriter, r *http.Request) {
	userID := s.currentUserID(r)
	creds, err := s.passkeys.ListByUser(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list passkeys")
		return
	}
	type passkeyMeta struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	out := make([]passkeyMeta, 0, len(creds))
	for _, c := range creds {
		out = append(out, passkeyMeta{ID: auth.CredentialIDString(c.ID), Name: "passkey"})
	}
	writeJSON(w, http.StatusOK, out)
}

// handlePasskeyDelete removes a passkey by credential ID.
func (s *Server) handlePasskeyDelete(w http.ResponseWriter, r *http.Request) {
	userID := s.currentUserID(r)
	var req struct {
		ID string `json:"id"`
	}
	if err := decodeJSON(w, r, &req); err != nil || req.ID == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "passkey id is required")
		return
	}
	credID, err := auth.ParseCredentialIDString(req.ID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid passkey id")
		return
	}
	if err := s.passkeys.Delete(r.Context(), userID, credID); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not delete passkey")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
