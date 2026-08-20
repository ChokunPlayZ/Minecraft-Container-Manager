// Package api implements the HTTP REST API and static file serving for MCM.
package api

import (
	"context"
	"encoding/json"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strings"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/backups"
	"github.com/mcm-panel/mcm/internal/config"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/dns"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
	"github.com/mcm-panel/mcm/internal/spindown"
	"github.com/mcm-panel/mcm/internal/web"
)

// Server wires the API handlers and middleware together.
type Server struct {
	cfg        *config.Config
	db         *db.Store
	servers    *servers.Store
	backups    *backups.Store
	users      *auth.Users
	sessions   *auth.Manager
	passkeys   *auth.Passkeys
	webAuthn   *webauthn.WebAuthn
	ceremonies *ceremonyStore
	jars       *jars.Resolver
	dns        *dns.Service
	spin       *spindown.Service
	logger     *log.Logger
	mux        *http.ServeMux
	loginLimit *loginLimiter
}

// Config holds the dependencies required to build the API server.
type Options struct {
	Cfg      *config.Config
	DB       *db.Store
	Servers  *servers.Store
	Backups  *backups.Store
	Users    *auth.Users
	Sessions *auth.Manager
	Passkeys *auth.Passkeys
	WebAuthn *webauthn.WebAuthn
	Jars     *jars.Resolver
	DNS      *dns.Service
	Spin     *spindown.Service
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
		cfg:        opts.Cfg,
		db:         opts.DB,
		servers:    opts.Servers,
		backups:    opts.Backups,
		users:      opts.Users,
		sessions:   opts.Sessions,
		passkeys:   opts.Passkeys,
		webAuthn:   opts.WebAuthn,
		ceremonies: newCeremonyStore(),
		jars:       opts.Jars,
		dns:        opts.DNS,
		spin:       opts.Spin,
		logger:     opts.Logger,
		mux:        http.NewServeMux(),
		loginLimit: newLoginLimiter(loginDefaults(opts.Cfg)),
	}
	s.routes()
	return s.withMiddleware(s.mux)
}

func (s *Server) routes() {
	// Health and readiness (unauthenticated).
	s.mux.HandleFunc("GET /healthz", s.handleHealthz)
	s.mux.HandleFunc("GET /readyz", s.handleReadyz)

	// Auth and onboarding.
	s.mux.HandleFunc("GET /api/auth/csrf", s.wrapJSON(s.handleCSRF))
	s.mux.HandleFunc("GET /api/onboarding/status", s.wrapJSON(s.handleOnboardingStatus))
	s.mux.HandleFunc("POST /api/onboarding", s.wrapJSON(s.handleOnboarding))
	s.mux.HandleFunc("POST /api/auth/login", s.wrapJSON(s.handleLogin))
	s.mux.HandleFunc("POST /api/auth/logout", s.requireAuth(s.wrapJSON(s.handleLogout)))
	s.mux.HandleFunc("GET /api/auth/me", s.requireAuth(s.wrapJSON(s.handleMe)))

	// TOTP two-factor authentication.
	s.mux.HandleFunc("GET /api/auth/totp", s.requireAuth(s.wrapJSON(s.handleTOTPStatus)))
	s.mux.HandleFunc("POST /api/auth/totp/enroll", s.requireAuth(s.wrapJSON(s.handleTOTPEnroll)))
	s.mux.HandleFunc("POST /api/auth/totp/enroll/confirm", s.requireAuth(s.wrapJSON(s.handleTOTPConfirm)))
	s.mux.HandleFunc("POST /api/auth/totp/disable", s.requireAuth(s.wrapJSON(s.handleTOTPDisable)))

	// Passkey (WebAuthn) enrollment and login.
	s.mux.HandleFunc("POST /api/passkey/register/begin", s.requireAuth(s.wrapJSON(s.handlePasskeyRegisterBegin)))
	s.mux.HandleFunc("POST /api/passkey/register/finish", s.requireAuth(s.wrapJSON(s.handlePasskeyRegisterFinish)))
	s.mux.HandleFunc("GET /api/passkey", s.requireAuth(s.wrapJSON(s.handlePasskeyList)))
	s.mux.HandleFunc("DELETE /api/passkey", s.requireAuth(s.wrapJSON(s.handlePasskeyDelete)))
	s.mux.HandleFunc("POST /api/passkey/login/begin", s.wrapJSON(s.handlePasskeyLoginBegin))
	s.mux.HandleFunc("POST /api/passkey/login/finish", s.wrapJSON(s.handlePasskeyLoginFinish))

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

	// Server management (players, ops, mods/plugins).
	s.mux.HandleFunc("GET /api/servers/{id}/players", s.requireAuth(s.wrapJSON(s.handleListPlayers)))
	s.mux.HandleFunc("POST /api/servers/{id}/players/{name}/command", s.requireAuth(s.wrapJSON(s.handleRunPlayerCommand)))
	s.mux.HandleFunc("GET /api/servers/{id}/ops", s.requireAuth(s.wrapJSON(s.handleListOps)))
	s.mux.HandleFunc("POST /api/servers/{id}/ops", s.requireAuth(s.wrapJSON(s.handleAddOP)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/ops/{name}", s.requireAuth(s.wrapJSON(s.handleRemoveOP)))
	s.mux.HandleFunc("GET /api/servers/{id}/mods", s.requireAuth(s.wrapJSON(s.handleListMods)))
	s.mux.HandleFunc("POST /api/servers/{id}/mods", s.requireAuth(s.wrapJSON(s.handleUploadMod)))
	s.mux.HandleFunc("PATCH /api/servers/{id}/mods/{name}", s.requireAuth(s.wrapJSON(s.handleSetModEnabled)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/mods/{name}", s.requireAuth(s.wrapJSON(s.handleDeleteMod)))
	s.mux.HandleFunc("GET /api/servers/{id}/properties", s.requireAuth(s.wrapJSON(s.handleGetProperties)))
	s.mux.HandleFunc("PUT /api/servers/{id}/properties", s.requireAuth(s.wrapJSON(s.handleSaveProperties)))

	// File manager.
	s.mux.HandleFunc("GET /api/servers/{id}/files", s.requireAuth(s.wrapJSON(s.handleListFiles)))
	s.mux.HandleFunc("GET /api/servers/{id}/files/download", s.requireAuth(s.handleDownloadFile))
	s.mux.HandleFunc("POST /api/servers/{id}/files/upload", s.requireAuth(s.wrapJSON(s.handleUploadFile)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/archive", s.requireAuth(s.wrapJSON(s.handleArchiveFile)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/unzip", s.requireAuth(s.wrapJSON(s.handleUnzipFile)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/from_url", s.requireAuth(s.wrapJSON(s.handleDownloadFromURL)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/files", s.requireAuth(s.wrapJSON(s.handleDeleteFile)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/mkdir", s.requireAuth(s.wrapJSON(s.handleMkdir)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/rename", s.requireAuth(s.wrapJSON(s.handleRenameFile)))

	// Idle spin-down.
	s.mux.HandleFunc("GET /api/spindown", s.requireAuth(s.wrapJSON(s.handleListSpindown)))
	s.mux.HandleFunc("POST /api/servers/{id}/wake", s.requireAuth(s.wrapJSON(s.handleWakeServer)))
	s.mux.HandleFunc("GET /api/servers/{id}/spindown", s.requireAuth(s.wrapJSON(s.handleGetServerSpindown)))
	s.mux.HandleFunc("PUT /api/servers/{id}/spindown", s.requireAuth(s.wrapJSON(s.handlePutServerSpindown)))
	s.mux.HandleFunc("POST /api/servers/{id}/activity", s.requireAuth(s.wrapJSON(s.handleServerActivity)))

	// Backups.
	s.mux.HandleFunc("POST /api/servers/{id}/backup", s.requireAuth(s.wrapJSON(s.handleBackupServer)))
	s.mux.HandleFunc("GET /api/servers/{id}/backups", s.requireAuth(s.wrapJSON(s.handleListBackups)))
	s.mux.HandleFunc("POST /api/servers/{id}/restore/{backupId}", s.requireAuth(s.wrapJSON(s.handleRestoreBackup)))
	s.mux.HandleFunc("DELETE /api/backups/{backupId}", s.requireAuth(s.wrapJSON(s.handleDeleteBackup)))

	// Jars.
	s.mux.HandleFunc("GET /api/jars/{kind}/versions", s.requireAuth(s.wrapJSON(s.handleJarVersions)))
	s.mux.HandleFunc("GET /api/jars/{kind}/versions/{v}/builds", s.requireAuth(s.wrapJSON(s.handleJarBuilds)))

	// User management.
	s.mux.HandleFunc("GET /api/users", s.requireAuth(s.wrapJSON(s.handleListUsers)))
	s.mux.HandleFunc("POST /api/users", s.requireAuth(s.wrapJSON(s.handleCreateUser)))
	s.mux.HandleFunc("PATCH /api/users/{id}", s.requireAuth(s.wrapJSON(s.handleUpdateUser)))
	s.mux.HandleFunc("DELETE /api/users/{id}", s.requireAuth(s.wrapJSON(s.handleDeleteUser)))

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
	var h http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if !csrfExempt(r) && !csrfTokenValid(r) {
			writeError(w, http.StatusForbidden, "csrf_missing", "missing or invalid CSRF token")
			return
		}
		next.ServeHTTP(w, r)
	})
	h = s.rateLimit(h)
	return s.logRequests(h)
}

// csrfExempt reports whether a request is allowed to skip CSRF validation.
// Only safe (read) methods and the unauthenticated onboarding/login endpoints
// are exempt.
func csrfExempt(r *http.Request) bool {
	if csrfSafeMethods[r.Method] {
		return true
	}
	return csrfExemptPaths[r.URL.Path]
}

// csrfTokenValid verifies the double-submit cookie matches the header token.
func csrfTokenValid(r *http.Request) bool {
	cookie, err := r.Cookie(CSRFCookieName)
	if err != nil {
		return false
	}
	return csrfTokenMatches(cookie.Value, r.Header.Get(csrfHeaderName))
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
		// SPA fallback: serve index.html for client-side navigation routes.
		// Resource-looking paths (with a file extension) that are missing are
		// genuine 404s, not navigation routes; do not soft-404 them.
		if isAssetPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
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

// isAssetPath reports whether a request path looks like a static resource (a
// path whose base name has a file extension) as opposed to a client-side
// navigation route. Navigation routes are the only ones that should receive
// the SPA index.html fallback.
func isAssetPath(p string) bool {
	return strings.Contains(path.Base(p), ".")
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
