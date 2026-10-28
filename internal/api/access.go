package api

import (
	"net/http"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/servers"
)

// Admins may use every route. Members may use their own account routes and
// the routes of servers assigned to them; other servers answer 404 so members
// cannot discover them.

func (s *Server) isAdmin(r *http.Request) bool {
	role, _ := r.Context().Value(ctxRole).(string)
	return role == auth.RoleAdmin
}

// canAccessServer reports whether the current user may manage serverID.
func (s *Server) canAccessServer(r *http.Request, serverID string) (bool, error) {
	if s.isAdmin(r) {
		return true, nil
	}
	return s.users.HasServerAccess(r.Context(), s.currentUserID(r), serverID)
}

// requireAdmin allows only administrators.
func (s *Server) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		if !s.isAdmin(r) {
			writeError(w, http.StatusForbidden, "forbidden", "Only administrators can do this.")
			return
		}
		next(w, r)
	})
}

// requireServer allows administrators and members assigned to the {id} server.
func (s *Server) requireServer(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		ok, err := s.canAccessServer(r, r.PathValue("id"))
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not check server access")
			return
		}
		if !ok {
			writeError(w, http.StatusNotFound, "not_found", "server not found")
			return
		}
		next(w, r)
	})
}

// authorizeBackup writes an error and returns false unless the current user
// may manage the server that owns backupID.
func (s *Server) authorizeBackup(w http.ResponseWriter, r *http.Request, backupID string) bool {
	if s.isAdmin(r) {
		return true
	}
	b, err := s.backups.Get(r.Context(), backupID)
	if err != nil {
		s.writeBackupErr(w, err)
		return false
	}
	ok, err := s.canAccessServer(r, b.ServerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not check server access")
		return false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "not_found", "backup not found")
		return false
	}
	return true
}

// filterServers keeps the servers whose ids are in allowed, in list order.
func filterServers(list []servers.Server, allowed []string) []servers.Server {
	keep := make(map[string]bool, len(allowed))
	for _, id := range allowed {
		keep[id] = true
	}
	out := make([]servers.Server, 0, len(allowed))
	for _, srv := range list {
		if keep[srv.ID] {
			out = append(out, srv)
		}
	}
	return out
}
