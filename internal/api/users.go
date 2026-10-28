package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/mcm-panel/mcm/internal/auth"
)

const minPasswordLen = 8

// User management is admin-only (see routes), except that members may update
// their own email and password. Admins manage everything; members manage only
// the servers listed in their server_ids.

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	// Role defaults to admin, matching accounts created before roles existed.
	Role      string   `json:"role"`
	ServerIDs []string `json:"server_ids"`
}

type updateUserRequest struct {
	Email     *string   `json:"email"`
	Password  *string   `json:"password"`
	Role      *string   `json:"role"`
	ServerIDs *[]string `json:"server_ids"`
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
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "invalid_request", "email is required")
		return
	}
	if len(req.Password) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters")
		return
	}
	if req.Role == "" {
		req.Role = auth.RoleAdmin
	}
	if !auth.ValidRole(req.Role) {
		writeError(w, http.StatusBadRequest, "invalid_request", "role must be admin or member")
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
	user, err := s.users.CreateWithRole(r.Context(), req.Email, hash, req.Role)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not create user")
		return
	}
	if req.Role == auth.RoleMember && len(req.ServerIDs) > 0 {
		if err := s.users.SetServerAccess(r.Context(), user.ID, req.ServerIDs); err != nil {
			// Do not leave a half-configured account behind.
			_ = s.users.Delete(r.Context(), user.ID)
			s.writeServerAccessErr(w, err)
			return
		}
		if user, err = s.users.GetByID(r.Context(), user.ID); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not load created user")
			return
		}
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
	if req.Email == nil && req.Password == nil && req.Role == nil && req.ServerIDs == nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "nothing to update")
		return
	}
	// Members may change only their own email and password.
	if !s.isAdmin(r) && (id != s.currentUserID(r) || req.Role != nil || req.ServerIDs != nil) {
		writeError(w, http.StatusForbidden, "forbidden", "Only administrators can do this.")
		return
	}

	target, err := s.users.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusNotFound, "not_found", "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not look up user")
		return
	}
	role := target.Role
	if req.Role != nil && *req.Role != target.Role {
		if !auth.ValidRole(*req.Role) {
			writeError(w, http.StatusBadRequest, "invalid_request", "role must be admin or member")
			return
		}
		// Only admins reach this handler, so blocking self-changes also keeps
		// at least one admin.
		if id == s.currentUserID(r) {
			writeError(w, http.StatusBadRequest, "invalid_request", "You cannot change your own role")
			return
		}
		role = *req.Role
	}

	var hash *string
	if req.Email != nil {
		email := strings.ToLower(strings.TrimSpace(*req.Email))
		if email == "" {
			writeError(w, http.StatusBadRequest, "invalid_request", "email cannot be empty")
			return
		}
		req.Email = &email
	}
	if req.Password != nil {
		if len(*req.Password) < minPasswordLen {
			writeError(w, http.StatusBadRequest, "invalid_request", "password must be at least 8 characters")
			return
		}
		value, err := auth.HashPassword(*req.Password)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not hash password")
			return
		}
		hash = &value
	}
	keepToken := ""
	if id == s.currentUserID(r) {
		if cookie, err := r.Cookie(auth.CookieName); err == nil {
			keepToken = cookie.Value
		}
	}
	if err := s.users.UpdateAccount(r.Context(), id, req.Email, hash, keepToken); err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "email_taken", "a user with that email already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", "could not update account")
		return
	}

	if role != target.Role {
		if err := s.users.UpdateRole(r.Context(), id, role); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not update role")
			return
		}
	}
	// Admins can reach every server, so they keep no assignments.
	if role == auth.RoleAdmin && len(target.ServerIDs) > 0 {
		if err := s.users.SetServerAccess(r.Context(), id, nil); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not update server access")
			return
		}
	} else if role == auth.RoleMember && req.ServerIDs != nil {
		if err := s.users.SetServerAccess(r.Context(), id, *req.ServerIDs); err != nil {
			s.writeServerAccessErr(w, err)
			return
		}
	}

	user, err := s.users.GetByID(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not load updated user")
		return
	}
	writeJSON(w, http.StatusOK, user)
}

func (s *Server) writeServerAccessErr(w http.ResponseWriter, err error) {
	if errors.Is(err, auth.ErrUnknownServer) {
		writeError(w, http.StatusBadRequest, "invalid_request", "One or more selected servers no longer exist")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal", "could not update server access")
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
