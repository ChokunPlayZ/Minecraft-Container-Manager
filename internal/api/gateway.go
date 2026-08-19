package api

import (
	"context"
	"net/http"
)

// gatewayActive reports the effective gateway activation state, honoring the
// MCM_GATEWAY env knob and (in auto mode) the live gateway_enabled setting.
func (s *Server) gatewayActive() (bool, error) {
	if s.cfg != nil {
		switch s.cfg.Gateway {
		case "on":
			return true, nil
		case "off":
			return false, nil
		}
	}
	if s.servers == nil {
		return false, nil
	}
	return s.servers.GatewayEnabled(context.Background())
}

// handleGetGateway returns a server's gateway configuration and last-known-good
// MOTD.
func (s *Server) handleGetGateway(w http.ResponseWriter, r *http.Request) {
	enabled, err := s.gatewayActive()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not read gateway settings")
		return
	}
	info, err := s.servers.GatewayInfo(r.Context(), r.PathValue("id"), enabled)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handlePutGateway sets a server's per-server wake message.
func (s *Server) handlePutGateway(w http.ResponseWriter, r *http.Request) {
	var body struct {
		WakeMessage string `json:"wake_message"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if err := s.servers.SetWakeMessage(r.Context(), r.PathValue("id"), body.WakeMessage); err != nil {
		s.writeServerErr(w, err)
		return
	}
	enabled, err := s.gatewayActive()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not read gateway settings")
		return
	}
	info, err := s.servers.GatewayInfo(r.Context(), r.PathValue("id"), enabled)
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, info)
}
