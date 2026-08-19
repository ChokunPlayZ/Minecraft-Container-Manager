// Package api implements the HTTP REST API and static file serving for MCM.
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/backups"
	"github.com/mcm-panel/mcm/internal/config"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/dns"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
	"github.com/mcm-panel/mcm/internal/web"
)

// Server wires the API handlers and middleware together.
type Server struct {
	cfg      *config.Config
	db       *db.Store
	servers  *servers.Store
	backups  *backups.Store
	users    *auth.Users
	sessions *auth.Manager
	jars     *jars.Resolver
	dns      *dns.Service
	logger   *log.Logger
	mux      *http.ServeMux
}

// Config holds the dependencies required to build the API server.
type Options struct {
	Cfg      *config.Config
	DB       *db.Store
	Servers  *servers.Store
	Backups  *backups.Store
	Users    *auth.Users
	Sessions *auth.Manager
	Jars     *jars.Resolver
	DNS      *dns.Service
	Logger   *log.Logger
}

type ctxKey string

const (
	ctxUserID ctxKey = "userID"
)

// New builds an http.Handler with all API routes registered.
func New(opts Options) http.Handler {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	s := &Server{
		cfg:      opts.Cfg,
		db:       opts.DB,
		servers:  opts.Servers,
		backups:  opts.Backups,
		users:    opts.Users,
		sessions: opts.Sessions,
		jars:     opts.Jars,
		dns:      opts.DNS,
		logger:   opts.Logger,
		mux:      http.NewServeMux(),
	}
	s.routes()
	return s.withMiddleware(s.mux)
}

func (s *Server) routes() {
	// Auth and onboarding.
	s.mux.HandleFunc("POST /api/onboarding", s.wrapJSON(s.handleOnboarding))
	s.mux.HandleFunc("POST /api/auth/login", s.wrapJSON(s.handleLogin))
	s.mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.wrapJSON(s.handleLogout)))
	s.mux.HandleFunc("GET /api/auth/me", s.requireAuth(s.wrapJSON(s.handleMe)))

	// Servers.
	s.mux.HandleFunc("GET /api/servers", s.requireAuth(s.wrapJSON(s.handleListServers)))
	s.mux.HandleFunc("POST /api/servers", s.requireAuth(s.wrapJSON(s.handleCreateServer)))
	s.mux.HandleFunc("GET /api/servers/{id}", s.requireAuth(s.wrapJSON(s.handleGetServer)))
	s.mux.HandleFunc("PATCH /api/servers/{id}", s.requireAuth(s.wrapJSON(s.handleUpdateServer)))
	s.mux.HandleFunc("DELETE /api/servers/{id}", s.requireAuth(s.wrapJSON(s.handleDeleteServer)))
	s.mux.HandleFunc("POST /api/servers/{id}/start", s.requireAuth(s.wrapJSON(s.handleServerAction("start"))))
	s.mux.HandleFunc("POST /api/servers/{id}/stop", s.requireAuth(s.wrapJSON(s.handleServerAction("stop"))))
	s.mux.HandleFunc("POST /api/servers/{id}/restart", s.requireAuth(s.wrapJSON(s.handleServerAction("restart"))))
	s.mux.HandleFunc("GET /api/servers/{id}/status", s.requireAuth(s.wrapJSON(s.handleServerStatus)))
	s.mux.HandleFunc("GET /api/servers/{id}/console", s.requireAuth(s.handleServerConsole))
	s.mux.HandleFunc("GET /api/servers/{id}/install", s.requireAuth(s.wrapJSON(s.handleInstall(false))))
	s.mux.HandleFunc("POST /api/servers/{id}/install", s.requireAuth(s.wrapJSON(s.handleInstall(true))))

	// Backups.
	s.mux.HandleFunc("POST /api/servers/{id}/backup", s.requireAuth(s.wrapJSON(s.handleBackupServer)))
	s.mux.HandleFunc("GET /api/servers/{id}/backups", s.requireAuth(s.wrapJSON(s.handleListBackups)))
	s.mux.HandleFunc("POST /api/servers/{id}/restore/{backupId}", s.requireAuth(s.wrapJSON(s.handleRestoreBackup)))
	s.mux.HandleFunc("DELETE /api/backups/{backupId}", s.requireAuth(s.wrapJSON(s.handleDeleteBackup)))

	// Jars.
	s.mux.HandleFunc("GET /api/jars/{kind}/versions", s.requireAuth(s.wrapJSON(s.handleJarVersions)))
	s.mux.HandleFunc("GET /api/jars/{kind}/versions/{v}/builds", s.requireAuth(s.wrapJSON(s.handleJarBuilds)))

	// Ports and settings.
	s.mux.HandleFunc("GET /api/ports/available", s.requireAuth(s.wrapJSON(s.handleAvailablePorts)))
	s.mux.HandleFunc("GET /api/settings", s.requireAuth(s.wrapJSON(s.handleGetSettings)))
	s.mux.HandleFunc("PUT /api/settings", s.requireAuth(s.wrapJSON(s.handlePutSettings)))

	// DNS publishing.
	s.mux.HandleFunc("GET /api/dns", s.requireAuth(s.wrapJSON(s.handleListDNS)))
	s.mux.HandleFunc("POST /api/servers/{id}/dns", s.requireAuth(s.wrapJSON(s.handlePublishDNS)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/dns", s.requireAuth(s.wrapJSON(s.handleRemoveDNS)))

	// Static frontend.
	s.mux.HandleFunc("/", s.handleStatic)
}

func (s *Server) withMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) wrapJSON(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		h(w, r)
	}
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(auth.CookieName)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "authentication required")
			return
		}
		userID, err := s.sessions.Validate(r.Context(), cookie.Value)
		if err != nil {
			http.SetCookie(w, &http.Cookie{Name: auth.CookieName, Value: "", MaxAge: -1, Path: "/"})
			writeError(w, http.StatusUnauthorized, "unauthorized", "session is invalid or expired")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) currentUserID(r *http.Request) string {
	if v, ok := r.Context().Value(ctxUserID).(string); ok {
		return v
	}
	return ""
}

func (s *Server) handleStatic(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		writeError(w, http.StatusNotFound, "not_found", "route not found")
		return
	}
	sub, err := fs.Sub(web.Dist, "dist")
	if err != nil {
		http.Error(w, "embedded frontend unavailable", http.StatusInternalServerError)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/")
	if path == "" {
		path = "index.html"
	}
	if _, err := fs.Stat(sub, path); err != nil {
		// SPA fallback: serve index.html for client-side routes.
		path = "index.html"
	}
	data, err := fs.ReadFile(sub, path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if ct := mime.TypeByExtension(filepath.Ext(path)); ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	_, _ = w.Write(data)
}

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	e := apiError{}
	e.Error.Code = code
	e.Error.Message = message
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(e)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	return dec.Decode(v)
}
