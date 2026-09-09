package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

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

	cfg, _ := s.dns.GetConfig(r.Context())
	configured := cfg != nil && cfg.Publish && cfg.HasToken && cfg.Domain != "" && cfg.Zone != ""

	writeJSON(w, http.StatusOK, map[string]any{
		"records":    records,
		"config":     cfg,
		"configured": configured,
	})
}

// handleGetServerDNS returns the DNS record and computed join address for a specific server.
func (s *Server) handleGetServerDNS(w http.ResponseWriter, r *http.Request) {
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

	cfg, _ := s.dns.GetConfig(r.Context())
	configured := cfg != nil && cfg.Publish && cfg.HasToken && cfg.Domain != "" && cfg.Zone != ""

	rec, _ := s.dns.GetRecord(r.Context(), id)

	var joinAddress string
	if rec != nil && cfg != nil && cfg.Domain != "" {
		if rec.Subdomain == "@" || rec.Subdomain == "" {
			joinAddress = cfg.Domain
		} else {
			joinAddress = rec.Subdomain + "." + cfg.Domain
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"record":       rec,
		"configured":   configured,
		"domain":       cfg.Domain,
		"server_id":    srv.ID,
		"server_name":  srv.Name,
		"host_port":    srv.HostPort,
		"join_address": joinAddress,
	})
}

type publishDNSRequest struct {
	Subdomain string `json:"subdomain"`
	Target    string `json:"target"`
	Port      int    `json:"port"`
	Priority  *int   `json:"priority"`
	Weight    *int   `json:"weight"`
}

// handlePublishDNS creates or updates the SRV record that points a domain at a
// server.
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

	var req publishDNSRequest
	// Body is optional
	if r.Body != nil {
		bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &req)
		}
	}

	port := req.Port
	if port <= 0 {
		port = srv.HostPort
	}

	rec, err := s.dns.UpsertWithOptions(r.Context(), id, dns.PublishOptions{
		Subdomain: req.Subdomain,
		Host:      req.Target,
		Port:      port,
		Priority:  req.Priority,
		Weight:    req.Weight,
	})
	if err != nil {
		if errors.Is(err, dns.ErrNotConfigured) {
			writeError(w, http.StatusConflict, "not_configured", "DNS publishing is not configured or disabled")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	cfg, _ := s.dns.GetConfig(r.Context())
	var joinAddress string
	if rec != nil && cfg != nil && cfg.Domain != "" {
		if rec.Subdomain == "@" || rec.Subdomain == "" {
			joinAddress = cfg.Domain
		} else {
			joinAddress = rec.Subdomain + "." + cfg.Domain
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":           true,
		"record":       rec,
		"join_address": joinAddress,
	})
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

type testDNSRequest struct {
	APIToken string `json:"api_token"`
	Zone     string `json:"zone"`
	Domain   string `json:"domain"`
}

// handleTestDNS verifies Cloudflare credentials and reports connectivity.
func (s *Server) handleTestDNS(w http.ResponseWriter, r *http.Request) {
	if s.dns == nil {
		writeError(w, http.StatusNotFound, "not_found", "DNS publishing is not enabled")
		return
	}

	var req testDNSRequest
	if r.Body != nil {
		bodyBytes, _ := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if len(bodyBytes) > 0 {
			_ = json.Unmarshal(bodyBytes, &req)
		}
	}

	result, err := s.dns.Verify(r.Context(), strings.TrimSpace(req.APIToken), strings.TrimSpace(req.Zone), strings.TrimSpace(req.Domain))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", err.Error())
		return
	}

	writeJSON(w, http.StatusOK, result)
}
