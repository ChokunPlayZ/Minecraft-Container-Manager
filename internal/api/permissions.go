package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/mcm-panel/mcm/internal/auth"
)

func (s *Server) currentUser(r *http.Request) auth.User {
	user, _ := r.Context().Value(ctxUser).(auth.User)
	return user
}

func (s *Server) requirePermission(permission string, next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		user := s.currentUser(r)
		serverID := ""
		if strings.Contains(r.Pattern, "/api/servers/{id}") {
			serverID = r.PathValue("id")
		}
		// Resolve indirect resource ownership before opening or deleting an archive.
		if strings.Contains(r.Pattern, "/api/backups/{backupId}") {
			if !user.Has(permission) {
				permissionDenied(w)
				return
			}
			backup, err := s.backups.Get(r.Context(), r.PathValue("backupId"))
			if err != nil {
				s.writeBackupErr(w, err)
				return
			}
			serverID = backup.ServerID
		}
		if !user.Can(permission, serverID) {
			permissionDenied(w)
			return
		}
		if permission == "servers.copy" && (!user.Has("servers.create") || !user.Has("files.read") || !user.AllServers) {
			permissionDenied(w)
			return
		}
		// Long-lived event streams must also lose access after a permission,
		// scope, session or account change, without waiting for reconnection.
		if r.Method == http.MethodGet && (strings.HasSuffix(r.Pattern, "/events") || strings.HasSuffix(r.Pattern, "/console")) {
			cookie, _ := r.Cookie(auth.CookieName)
			ctx, cancel := context.WithCancel(r.Context())
			defer cancel()
			go func() {
				ticker := time.NewTicker(5 * time.Second)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						id, err := s.sessions.Validate(ctx, cookie.Value)
						if err != nil {
							cancel()
							return
						}
						fresh, err := s.users.GetByID(ctx, id)
						if err != nil || !fresh.Can(permission, serverID) {
							cancel()
							return
						}
					}
				}
			}()
			r = r.WithContext(ctx)
		}
		next(w, r)
	})
}

func permissionDenied(w http.ResponseWriter) {
	writeError(w, http.StatusForbidden, "forbidden", "You do not have permission to perform this action.")
}

func (s *Server) handlePermissionCatalog(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.DB.QueryContext(r.Context(), `SELECT id, name FROM servers ORDER BY name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list server assignments")
		return
	}
	defer rows.Close()
	servers := []map[string]string{}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not list server assignments")
			return
		}
		servers = append(servers, map[string]string{"id": id, "name": name})
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list server assignments")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"permissions": auth.PermissionCatalog, "roles": auth.RolePresets(), "servers": servers})
}

func (s *Server) validateAccess(w http.ResponseWriter, r *http.Request, access *auth.Access) bool {
	if err := access.Normalize(); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_permissions", err.Error())
		return false
	}
	for _, id := range access.ServerIDs {
		var found int
		if err := s.db.DB.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM servers WHERE id=?`, id).Scan(&found); err != nil {
			writeError(w, http.StatusInternalServerError, "internal", "could not validate server assignments")
			return false
		}
		if found == 0 {
			writeError(w, http.StatusBadRequest, "invalid_permissions", "a selected server does not exist")
			return false
		}
	}
	return true
}

// Port metadata is needed both when creating and editing server configuration.
// Existing-server requests must name an accessible server with edit access.
func (s *Server) requirePortAccess(next http.HandlerFunc) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		user := s.currentUser(r)
		id := r.URL.Query().Get("server_id")
		if !user.Has("servers.create") && (id == "" || !user.Can("servers.edit", id)) {
			permissionDenied(w)
			return
		}
		next(w, r)
	})
}

// requireAnyPermission is for global endpoints shared by readers and managers.
func (s *Server) requireAnyPermission(next http.HandlerFunc, permissions ...string) http.HandlerFunc {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		for _, permission := range permissions {
			if s.currentUser(r).Has(permission) {
				next(w, r)
				return
			}
		}
		permissionDenied(w)
	})
}
