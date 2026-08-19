package api

import (
	"errors"
	"net/http"

	"github.com/mcm-panel/mcm/internal/dns"
)

// handleListDNS returns the tracked DNS records along with the current
// publishing configuration status.
func (s *Server) handleListDNS(w http.ResponseWriter, r *http.Request) {
	if s.dns == nil {
		writeError(w, http.StatusNotFound, "not_found", "DNS publishing is not enabled")
		return
	}
	records, err := s.dns.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not list DNS records")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"records": records})
}

// handlePublishDNS creates or updates the SRV record that points a domain at a
// running server.
func (s *Server) handlePublishDNS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.dns == nil {
		writeError(w, http.StatusNotFound, "not_found", "DNS publishing is not enabled")
		return
	}
	srv, err := s.servers.Get(r.Context(), id)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	if err := s.dns.Upsert(r.Context(), id, "", srv.HostPort); err != nil {
		if errors.Is(err, dns.ErrNotConfigured) {
			writeError(w, http.StatusConflict, "not_configured", "DNS publishing is not configured")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleRemoveDNS deletes the SRV record for a server, if one was published.
func (s *Server) handleRemoveDNS(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.dns == nil {
		writeError(w, http.StatusNotFound, "not_found", "DNS publishing is not enabled")
		return
	}
	if err := s.dns.Remove(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
