package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/mcm-panel/mcm/internal/auth"
)

const minPasswordLen = 8

// Every authenticated user currently has the same privileges; user management
// is treated as an admin capability for all authenticated session holders. A
// dedicated RBAC system is intentionally out of scope.

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type updateUserRequest struct {
	Email    *string `json:"email"`
	Password *string `json:"password"`
}

// handleListUsers returns all panel users.
func (s *Server) handleListUsers(w http.ResponseWriter, r *http.Request) {
	users, err := s.users.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list users")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"users": users})
}

// handleCreateUser creates a new user with an email and password.
func (s *Server) handleCreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	req.Email = strings.TrimSpace(req.Email)
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "email is required")
		return
	}
	if len(req.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters")
		return
	}
	if _, err := s.users.GetByEmail(r.Context(), req.Email); err == nil {
		writeError(w, http.StatusConflict, "email_taken", "a user with that email already exists")
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
	writeJSON(w, http.StatusCreated, user)
}

// handleUpdateUser updates a user's email and/or password. Changing a password
// revokes the user's other sessions but preserves the acting admin's own
// session when the target is themself.
func (s *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req updateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if req.Email == nil && req.Password == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "nothing to update")
		return
	}

	if _, err := s.users.GetByID(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not look up user")
		return
	}

	if req.Email != nil {
		email := strings.TrimSpace(*req.Email)
		if email == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "email cannot be empty")
			return
		}
		if err := s.users.UpdateEmail(r.Context(), id, email); err != nil {
			if isUniqueViolation(err) {
				writeError(w, http.StatusConflict, "email_taken", "a user with that email already exists")
				return
			}
			writeError(w, http.StatusInternalServerError, "internal", "could not update email")
			return
		}
	}

	if req.Password != nil {
		if len(*req.Password) < minPasswordLen {
			writeError(w, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters")
			return
		}
		hash, err := auth.HashPassword(*req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not hash password")
			return
		}
		if err := s.users.UpdatePassword(r.Context(), id, hash); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not update password")
			return
		}

		if id == s.currentUserID(r) {
			// Preserve the acting admin's current session.
			if cookie, err := r.Cookie(auth.CookieName); err == nil {
				_ = s.sessions.RevokeByUserExcept(r.Context(), id, cookie.Value)
			}
		} else {
			_ = s.sessions.RevokeByUser(r.Context(), id)
		}
	}

	user, err := s.users.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load updated user")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

// handleDeleteUser deletes a user, their passkeys, and revokes their sessions.
func (s *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if id == s.currentUserID(r) {
		writeError(w, http.StatusBadRequest, "invalid_request", "You cannot delete your own account here")
		return
	}
	count, err := s.users.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not count users")
		return
	}
	if count <= 1 {
		writeError(w, http.StatusBadRequest, "invalid_request", "Cannot delete the last user")
		return
	}

	if err := s.users.Delete(r.Context(), id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not delete user")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "unique")
}
