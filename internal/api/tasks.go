package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// handleTaskProgress returns the current active task progress for a server, if any.
func (s *Server) handleTaskProgress(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	if _, err := s.servers.Get(r.Context(), serverID); err != nil {
		s.writeServerErr(w, err)
		return
	}

	prog := s.servers.GetTaskProgress(serverID)
	if prog == nil {
		writeJSON(w, http.StatusOK, map[string]any{"active": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"active":   true,
		"progress": prog,
	})
}

// handleTaskEvents streams Server-Sent Events (SSE) for task progress on a server.
func (s *Server) handleTaskEvents(w http.ResponseWriter, r *http.Request) {
	serverID := r.PathValue("id")
	if _, err := s.servers.Get(r.Context(), serverID); err != nil {
		s.writeServerErr(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "internal", "streaming unsupported")
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ch, cancel := s.servers.SubscribeTaskProgress(serverID)
	defer cancel()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case prog, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(prog)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
