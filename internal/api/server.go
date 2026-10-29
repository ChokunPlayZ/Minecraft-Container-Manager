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
	"github.com/mcm-panel/mcm/internal/notifications"
	"github.com/mcm-panel/mcm/internal/panelupdate"
	"github.com/mcm-panel/mcm/internal/proxy"
	"github.com/mcm-panel/mcm/internal/schedules"
	"github.com/mcm-panel/mcm/internal/servers"
	"github.com/mcm-panel/mcm/internal/web"
)

// Server wires the API handlers and middleware together.
type Server struct {
	cfg           *config.Config
	db            *db.Store
	servers       *servers.Store
	backups       *backups.Store
	schedules     *schedules.Store
	users         *auth.Users
	sessions      *auth.Manager
	passkeys      *auth.Passkeys
	webAuthn      *webauthn.WebAuthn
	ceremonies    *ceremonyStore
	jars          *jars.Resolver
	dns           *dns.Service
	proxy         *proxy.Service
	logger        *log.Logger
	notifications *notifications.Service
	panelUpdates  *panelupdate.Manager
	mux           *http.ServeMux
	loginLimit    *loginLimiter
}

// Config holds the dependencies required to build the API server.
type Options struct {
	Cfg           *config.Config
	DB            *db.Store
	Servers       *servers.Store
	Backups       *backups.Store
	Schedules     *schedules.Store
	Users         *auth.Users
	Sessions      *auth.Manager
	Passkeys      *auth.Passkeys
	WebAuthn      *webauthn.WebAuthn
	Jars          *jars.Resolver
	DNS           *dns.Service
	Logger        *log.Logger
	Notifications *notifications.Service
	PanelUpdates  *panelupdate.Manager
}

type ctxKey string

const (
	ctxUserID ctxKey = "userID"
	ctxRole   ctxKey = "role"
)

// New builds an http.Handler with all API routes registered.
func New(opts Options) http.Handler {
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.Schedules == nil && opts.DB != nil {
		opts.Schedules = schedules.NewStore(opts.DB.DB)
	}
	if opts.Notifications == nil && opts.DB != nil {
		opts.Notifications = notifications.New(opts.DB.DB, opts.Logger)
	}
	if opts.Servers != nil && opts.Backups != nil {
		opts.Backups.SetDataCoordinator(opts.Servers.WithStoppedData)
	}
	px := proxy.NewService()
	if opts.Servers != nil {
		opts.Servers.SetProxy(px)
	}
	s := &Server{
		cfg:           opts.Cfg,
		db:            opts.DB,
		servers:       opts.Servers,
		backups:       opts.Backups,
		schedules:     opts.Schedules,
		users:         opts.Users,
		sessions:      opts.Sessions,
		passkeys:      opts.Passkeys,
		webAuthn:      opts.WebAuthn,
		ceremonies:    newCeremonyStore(),
		jars:          opts.Jars,
		dns:           opts.DNS,
		proxy:         px,
		logger:        opts.Logger,
		notifications: opts.Notifications,
		panelUpdates:  opts.PanelUpdates,
		mux:           http.NewServeMux(),
		loginLimit:    newLoginLimiter(loginDefaults(opts.Cfg)),
	}
	s.routes()
	return s.withMiddleware(s.mux)
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/panel/update", s.requireAdmin(s.wrapJSON(s.handlePanelUpdateStatus)))
	s.mux.HandleFunc("POST /api/panel/update/check", s.requireAdmin(s.wrapJSON(s.handlePanelUpdateCheck)))
	s.mux.HandleFunc("POST /api/panel/update", s.requireAdmin(s.wrapJSON(s.handlePanelUpdateStart)))
	s.mux.HandleFunc("GET /api/notifications/discord", s.requireAdmin(s.wrapJSON(s.handleGetDiscord)))
	s.mux.HandleFunc("PUT /api/notifications/discord", s.requireAdmin(s.wrapJSON(s.handlePutDiscord)))
	s.mux.HandleFunc("POST /api/notifications/discord/test", s.requireAdmin(s.wrapJSON(s.handleTestDiscord)))
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
	s.mux.HandleFunc("POST /api/servers", s.requireAdmin(s.wrapJSON(s.handleCreateServer)))
	s.mux.HandleFunc("GET /api/servers/{id}", s.requireServer(s.wrapJSON(s.handleGetServer)))
	s.mux.HandleFunc("PATCH /api/servers/{id}", s.requireServer(s.wrapJSON(s.handleUpdateServer)))
	s.mux.HandleFunc("DELETE /api/servers/{id}", s.requireAdmin(s.wrapJSON(s.handleDeleteServer)))
	s.mux.HandleFunc("POST /api/servers/{id}/start", s.requireServer(s.wrapJSON(s.handleServerAction("start"))))
	s.mux.HandleFunc("POST /api/servers/{id}/stop", s.requireServer(s.wrapJSON(s.handleServerAction("stop"))))
	s.mux.HandleFunc("POST /api/servers/{id}/kill", s.requireServer(s.wrapJSON(s.handleServerAction("kill"))))
	s.mux.HandleFunc("POST /api/servers/{id}/restart", s.requireServer(s.wrapJSON(s.handleServerAction("restart"))))
	s.mux.HandleFunc("POST /api/servers/{id}/recreate", s.requireServer(s.wrapJSON(s.handleRecreateServer)))
	s.mux.HandleFunc("GET /api/servers/{id}/status", s.requireServer(s.wrapJSON(s.handleServerStatus)))
	s.mux.HandleFunc("GET /api/servers/{id}/stats", s.requireServer(s.wrapJSON(s.handleServerStats)))
	s.mux.HandleFunc("GET /api/servers/{id}/performance", s.requireServer(s.wrapJSON(s.handlePerformanceHistory)))
	s.mux.HandleFunc("PUT /api/servers/{id}/performance/settings", s.requireServer(s.wrapJSON(s.handlePerformanceCollection)))
	s.mux.HandleFunc("POST /api/servers/{id}/performance/spark", s.requireServer(s.wrapJSON(s.handleCaptureSparkReport)))
	s.mux.HandleFunc("GET /api/servers/{id}/console", s.requireServer(s.handleServerConsole))
	s.mux.HandleFunc("POST /api/servers/{id}/console/command", s.requireServer(s.wrapJSON(s.handleServerConsoleCommand)))
	s.mux.HandleFunc("GET /api/servers/{id}/install", s.requireServer(s.wrapJSON(s.handleInstall(false))))
	s.mux.HandleFunc("POST /api/servers/{id}/install", s.requireServer(s.wrapJSON(s.handleInstall(true))))
	s.mux.HandleFunc("GET /api/servers/{id}/export", s.requireServer(s.handleExportServer))
	s.mux.HandleFunc("POST /api/servers/{id}/copy", s.requireAdmin(s.wrapJSON(s.handleCopyServer)))

	// Server management (players, ops, mods/plugins).
	s.mux.HandleFunc("GET /api/servers/{id}/players", s.requireServer(s.wrapJSON(s.handleListPlayers)))
	s.mux.HandleFunc("GET /api/servers/{id}/players/data", s.requireServer(s.wrapJSON(s.handleListPlayerData)))
	s.mux.HandleFunc("POST /api/servers/{id}/players/{name}/command", s.requireServer(s.wrapJSON(s.handleRunPlayerCommand)))
	s.mux.HandleFunc("GET /api/servers/{id}/ops", s.requireServer(s.wrapJSON(s.handleListOps)))
	s.mux.HandleFunc("POST /api/servers/{id}/ops", s.requireServer(s.wrapJSON(s.handleAddOP)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/ops/{name}", s.requireServer(s.wrapJSON(s.handleRemoveOP)))
	s.mux.HandleFunc("GET /api/servers/{id}/whitelist", s.requireServer(s.wrapJSON(s.handleListWhitelist)))
	s.mux.HandleFunc("POST /api/servers/{id}/whitelist", s.requireServer(s.wrapJSON(s.handleAddWhitelist)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/whitelist/{name}", s.requireServer(s.wrapJSON(s.handleRemoveWhitelist)))
	s.mux.HandleFunc("GET /api/servers/{id}/mods", s.requireServer(s.wrapJSON(s.handleListMods)))
	s.mux.HandleFunc("GET /api/servers/{id}/mods/updates", s.requireServer(s.wrapJSON(s.handleGetModUpdates)))
	s.mux.HandleFunc("POST /api/servers/{id}/mods/updates/check", s.requireServer(s.wrapJSON(s.handleCheckModUpdates)))
	s.mux.HandleFunc("POST /api/servers/{id}/mods/batch-update", s.requireServer(s.wrapJSON(s.handleBatchUpdateMods)))
	s.mux.HandleFunc("GET /api/servers/{id}/mods/upgrade-check", s.requireServer(s.wrapJSON(s.handleCheckVersionUpgrade)))
	s.mux.HandleFunc("POST /api/servers/{id}/mods/upgrade-apply", s.requireServer(s.wrapJSON(s.handleApplyVersionUpgrade)))
	s.mux.HandleFunc("GET /api/servers/{id}/mods/{name}/versions", s.requireServer(s.wrapJSON(s.handleGetModVersions)))
	s.mux.HandleFunc("POST /api/servers/{id}/mods/{name}/update", s.requireServer(s.wrapJSON(s.handleUpdateMod)))
	s.mux.HandleFunc("POST /api/servers/{id}/mods", s.requireServer(s.wrapJSON(s.handleUploadMod)))
	s.mux.HandleFunc("POST /api/servers/{id}/mods/download", s.requireServer(s.wrapJSON(s.handleDownloadMod)))
	s.mux.HandleFunc("PATCH /api/servers/{id}/mods/{name}", s.requireServer(s.wrapJSON(s.handleSetModEnabled)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/mods/{name}", s.requireServer(s.wrapJSON(s.handleDeleteMod)))
	s.mux.HandleFunc("POST /api/servers/{id}/mods/sanitize-client-only", s.requireServer(s.wrapJSON(s.handleSanitizeClientMods)))

	// Modpack management.
	s.mux.HandleFunc("POST /api/modpack/inspect", s.requireAuth(s.wrapJSON(s.handleInspectModpack)))
	s.mux.HandleFunc("GET /api/servers/{id}/modpack", s.requireServer(s.wrapJSON(s.handleGetInstalledModpack)))
	s.mux.HandleFunc("POST /api/servers/{id}/modpack/inspect", s.requireServer(s.wrapJSON(s.handleInspectModpack)))
	s.mux.HandleFunc("POST /api/servers/{id}/modpack/install", s.requireServer(s.wrapJSON(s.handleInstallModpack)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/modpack", s.requireServer(s.wrapJSON(s.handleUninstallModpack)))

	// Server tasks (modpack installs, zipping, etc.)
	s.mux.HandleFunc("GET /api/servers/{id}/schedules", s.requireServer(s.wrapJSON(s.handleListSchedules)))
	s.mux.HandleFunc("POST /api/servers/{id}/schedules", s.requireServer(s.wrapJSON(s.handleCreateSchedule)))
	s.mux.HandleFunc("PUT /api/servers/{id}/schedules/{scheduleId}", s.requireServer(s.wrapJSON(s.handleUpdateSchedule)))
	s.mux.HandleFunc("PATCH /api/servers/{id}/schedules/{scheduleId}", s.requireServer(s.wrapJSON(s.handleToggleSchedule)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/schedules/{scheduleId}", s.requireServer(s.wrapJSON(s.handleDeleteSchedule)))
	s.mux.HandleFunc("GET /api/servers/{id}/tasks/progress", s.requireServer(s.wrapJSON(s.handleTaskProgress)))
	s.mux.HandleFunc("GET /api/servers/{id}/tasks/events", s.requireServer(s.handleTaskEvents))

	s.mux.HandleFunc("GET /api/servers/{id}/properties", s.requireServer(s.wrapJSON(s.handleGetProperties)))
	s.mux.HandleFunc("PUT /api/servers/{id}/properties", s.requireServer(s.wrapJSON(s.handleSaveProperties)))

	// Java world library, generation settings and chunk maintenance.
	s.mux.HandleFunc("GET /api/servers/{id}/worlds", s.requireServer(s.wrapJSON(s.handleListWorlds)))
	s.mux.HandleFunc("POST /api/servers/{id}/worlds", s.requireServer(s.wrapJSON(s.handleSaveWorld(true))))
	s.mux.HandleFunc("POST /api/servers/{id}/worlds/import", s.requireServer(s.wrapJSON(s.handleImportWorld)))
	s.mux.HandleFunc("PUT /api/servers/{id}/worlds/{world}", s.requireServer(s.wrapJSON(s.handleSaveWorld(false))))
	s.mux.HandleFunc("DELETE /api/servers/{id}/worlds/{world}", s.requireServer(s.wrapJSON(s.handleDeleteWorld)))
	s.mux.HandleFunc("POST /api/servers/{id}/worlds/{world}/activate", s.requireServer(s.wrapJSON(s.handleActivateWorld)))
	s.mux.HandleFunc("GET /api/servers/{id}/worlds/{world}/export", s.requireServer(s.handleExportWorld))
	s.mux.HandleFunc("GET /api/servers/{id}/worlds/{world}/regions", s.requireServer(s.wrapJSON(s.handleWorldRegions)))
	s.mux.HandleFunc("GET /api/servers/{id}/worlds/{world}/chunks", s.requireServer(s.wrapJSON(s.handleWorldChunks)))
	s.mux.HandleFunc("POST /api/servers/{id}/worlds/{world}/chunks/reset", s.requireServer(s.wrapJSON(s.handleResetWorldChunks)))

	// External catalog proxy.
	s.mux.HandleFunc("GET /api/proxy", s.requireAuth(s.handleProxy))
	s.mux.HandleFunc("POST /api/proxy", s.requireAuth(s.handleProxy))

	// Saved server logs, including compressed logs and tar archive members.
	s.mux.HandleFunc("GET /api/servers/{id}/logs", s.requireServer(s.wrapJSON(s.handleListLogs)))
	s.mux.HandleFunc("GET /api/servers/{id}/logs/content", s.requireServer(s.wrapJSON(s.handleReadLog)))

	// File manager.
	s.mux.HandleFunc("GET /api/servers/{id}/files", s.requireServer(s.wrapJSON(s.handleListFiles)))
	s.mux.HandleFunc("GET /api/servers/{id}/files/download", s.requireServer(s.handleDownloadFile))
	s.mux.HandleFunc("GET /api/servers/{id}/files/content", s.requireServer(s.wrapJSON(s.handleReadFileContent)))
	s.mux.HandleFunc("GET /api/servers/{id}/files/nbt", s.requireServer(s.wrapJSON(s.handleReadNBT)))
	s.mux.HandleFunc("PUT /api/servers/{id}/files/nbt", s.requireServer(s.wrapJSON(s.handleWriteNBT)))
	s.mux.HandleFunc("PUT /api/servers/{id}/files/content", s.requireServer(s.wrapJSON(s.handleWriteFileContent)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/upload", s.requireServer(s.wrapJSON(s.handleUploadFile)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/archive", s.requireServer(s.wrapJSON(s.handleArchiveFile)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/unzip", s.requireServer(s.wrapJSON(s.handleUnzipFile)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/from_url", s.requireServer(s.wrapJSON(s.handleDownloadFromURL)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/files", s.requireServer(s.wrapJSON(s.handleDeleteFile)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/mkdir", s.requireServer(s.wrapJSON(s.handleMkdir)))
	s.mux.HandleFunc("POST /api/servers/{id}/files/rename", s.requireServer(s.wrapJSON(s.handleRenameFile)))

	// Backups.
	s.mux.HandleFunc("POST /api/servers/{id}/backup", s.requireServer(s.wrapJSON(s.handleBackupServer)))
	s.mux.HandleFunc("GET /api/servers/{id}/backups", s.requireServer(s.wrapJSON(s.handleListBackups)))
	s.mux.HandleFunc("GET /api/servers/{id}/backups/progress", s.requireServer(s.wrapJSON(s.handleBackupProgress)))
	s.mux.HandleFunc("GET /api/servers/{id}/backups/events", s.requireServer(s.handleBackupEvents))
	s.mux.HandleFunc("POST /api/servers/{id}/backups/upload", s.requireServer(s.wrapJSON(s.handleUploadBackup)))
	s.mux.HandleFunc("POST /api/servers/{id}/restore/{backupId}", s.requireServer(s.wrapJSON(s.handleRestoreBackup)))
	s.mux.HandleFunc("DELETE /api/backups/{backupId}", s.requireAuth(s.wrapJSON(s.handleDeleteBackup)))
	s.mux.HandleFunc("GET /api/backups/{backupId}/download", s.requireAuth(s.handleDownloadBackup))

	// Jars and Java runtime.
	s.mux.HandleFunc("GET /api/system/java-versions", s.requireAuth(s.wrapJSON(s.handleSystemJavaVersions)))
	s.mux.HandleFunc("GET /api/system/capacity", s.requireAdmin(s.wrapJSON(s.handleHostCapacity)))
	s.mux.HandleFunc("GET /api/jars/{kind}/versions", s.requireAuth(s.wrapJSON(s.handleJarVersions)))
	s.mux.HandleFunc("GET /api/jars/{kind}/versions/{v}/builds", s.requireAuth(s.wrapJSON(s.handleJarBuilds)))

	// User management.
	s.mux.HandleFunc("GET /api/users", s.requireAdmin(s.wrapJSON(s.handleListUsers)))
	s.mux.HandleFunc("POST /api/users", s.requireAdmin(s.wrapJSON(s.handleCreateUser)))
	s.mux.HandleFunc("PATCH /api/users/{id}", s.requireAuth(s.wrapJSON(s.handleUpdateUser)))
	s.mux.HandleFunc("DELETE /api/users/{id}", s.requireAdmin(s.wrapJSON(s.handleDeleteUser)))

	// Ports and settings.
	s.mux.HandleFunc("GET /api/ports/available", s.requireAdmin(s.wrapJSON(s.handleAvailablePorts)))
	s.mux.HandleFunc("GET /api/ports/check", s.requireAdmin(s.wrapJSON(s.handleCheckPort)))
	s.mux.HandleFunc("GET /api/settings", s.requireAdmin(s.wrapJSON(s.handleGetSettings)))
	s.mux.HandleFunc("GET /api/updates", s.requireAuth(s.wrapJSON(s.handleGetUpdateChecker)))
	s.mux.HandleFunc("PUT /api/updates/settings", s.requireAdmin(s.wrapJSON(s.handlePutUpdateChecker)))
	s.mux.HandleFunc("POST /api/updates/check", s.requireAdmin(s.wrapJSON(s.handleCheckUpdates)))
	s.mux.HandleFunc("PUT /api/settings", s.requireAdmin(s.wrapJSON(s.handlePutSettings)))
	s.mux.HandleFunc("GET /api/docker/status", s.requireAdmin(s.wrapJSON(s.handleDockerStatus)))
	s.mux.HandleFunc("GET /api/runtime/status", s.requireAdmin(s.wrapJSON(s.handleDockerStatus)))

	// DNS publishing.
	s.mux.HandleFunc("GET /api/dns", s.requireAdmin(s.wrapJSON(s.handleListDNS)))
	s.mux.HandleFunc("POST /api/dns/test", s.requireAdmin(s.wrapJSON(s.handleTestDNS)))
	s.mux.HandleFunc("GET /api/servers/{id}/dns", s.requireServer(s.wrapJSON(s.handleGetServerDNS)))
	s.mux.HandleFunc("POST /api/servers/{id}/dns", s.requireAdmin(s.wrapJSON(s.handlePublishDNS)))
	s.mux.HandleFunc("DELETE /api/servers/{id}/dns", s.requireAdmin(s.wrapJSON(s.handleRemoveDNS)))

	// Every other /api route above uses requireAdmin, requireServer, or no
	// session at all. Plain requireAuth routes are open to members: their own
	// account, the filtered server list, update results, catalogs, jars, and
	// backups by id (checked in the handler).

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
		// Read the role on every request so role and access changes apply
		// immediately to existing sessions.
		role, err := s.users.Role(r.Context(), userID)
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "session is invalid or expired")
			return
		}
		ctx := context.WithValue(r.Context(), ctxUserID, userID)
		ctx = context.WithValue(ctx, ctxRole, role)
		next(w, r.WithContext(ctx))
	}
}

// logUpstream records the underlying upstream-provider failure before it is
// flattened into a generic 502 response. This keeps the specific provider URL
// and reason (status code, network error, parse failure) in the backend logs
// for diagnostics. It no-ops when no logger is configured (tests construct
// Server directly).
func (s *Server) logUpstream(err error, r *http.Request) {
	if s.logger == nil {
		return
	}
	if r != nil {
		s.logger.Printf("upstream provider failure method=%s path=%s err=%v", r.Method, r.URL.Path, err)
		return
	}
	s.logger.Printf("upstream provider failure err=%v", err)
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
	return decodeJSONLimit(w, r, v, 1<<20)
}
func decodeJSONLimit(w http.ResponseWriter, r *http.Request, v any, limit int64) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	return dec.Decode(v)
}
