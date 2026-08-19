package api

import (
	"bytes"
	"context"
	"io"
	"net/http"
)

// handleListSpindown reports idle spin-down state for every server.
func (s *Server) handleListSpindown(w http.ResponseWriter, r *http.Request) {
	if s.spin == nil {
		writeError(w, http.StatusServiceUnavailable, "disabled", "idle spin-down is not enabled")
		return
	}
	status, err := s.spin.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not read spin-down status")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"servers": status})
}

// handleWakeServer restarts a server that was stopped by idle spin-down.
func (s *Server) handleWakeServer(w http.ResponseWriter, r *http.Request) {
	if s.spin == nil {
		writeError(w, http.StatusServiceUnavailable, "disabled", "idle spin-down is not enabled")
		return
	}
	srv, err := s.spin.Wake(r.Context(), r.PathValue("id"))
	if err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, srv)
}

// handleServerActivity is the join-wake hook: recording activity wakes a
// stopped server and refreshes the idle clock of a running one.
func (s *Server) handleServerActivity(w http.ResponseWriter, r *http.Request) {
	if s.spin == nil {
		writeError(w, http.StatusServiceUnavailable, "disabled", "idle spin-down is not enabled")
		return
	}
	if err := s.spin.HandleJoin(r.Context(), r.PathValue("id")); err != nil {
		s.writeServerErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// handleGetServerSpindown reports idle spin-down state for a single server.
func (s *Server) handleGetServerSpindown(w http.ResponseWriter, r *http.Request) {
	if s.spin == nil {
		writeError(w, http.StatusServiceUnavailable, "disabled", "idle spin-down is not enabled")
		return
	}
	status, err := s.spin.Status(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal", "could not read spin-down status")
		return
	}
	id := r.PathValue("id")
	for _, st := range status {
		if st.ID == id {
			writeJSON(w, http.StatusOK, st)
			return
		}
	}
	writeError(w, http.StatusNotFound, "not_found", "server not found")
}

// handlePutServerSpindown sets or clears a server's idle timeout override.
func (s *Server) handlePutServerSpindown(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IdleTimeoutMinutes *int `json:"idle_timeout_minutes"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "invalid JSON body")
		return
	}
	if err := s.servers.SetIdleTimeoutOverride(r.Context(), r.PathValue("id"), body.IdleTimeoutMinutes); err != nil {
		s.writeServerErr(w, err)
		return
	}
	s.handleGetServerSpindown(w, r)
}

// configureConsoleJoinWatcher wraps an io.ReadCloser so that player join lines
// in the game log trigger a spin-down join-wake callback while the raw bytes
// still flow through unchanged. It is defensive and best-effort.
func (s *Server) configureConsoleJoinWatcher(ctx context.Context, id string, rc io.ReadCloser) io.ReadCloser {
	if s.spin == nil {
		return rc
	}
	return &joinWatchCloser{
		r: &joinWatcher{r: rc, id: id, onChange: func(id string) {
			_ = s.spin.HandleJoin(context.Background(), id)
		}},
		c: rc,
	}
}

// joinWatcher scans bytes for common Minecraft join messages and fires the
// callback once per detected join, passing bytes through unchanged.
type joinWatcher struct {
	r        io.Reader
	id       string
	onChange func(id string)
	scanned  []byte
	fired    bool
}

var joinPatterns = [][]byte{
	[]byte("joined the game"),
	[]byte("joined the lobby"),
}

func (j *joinWatcher) Read(p []byte) (int, error) {
	n, err := j.r.Read(p)
	if n > 0 {
		j.scanned = append(j.scanned, p[:n]...)
		if len(j.scanned) > 1<<20 {
			j.scanned = j.scanned[len(j.scanned)-4096:]
		}
		if !j.fired {
			lower := bytes.ToLower(j.scanned)
			for _, pat := range joinPatterns {
				if bytes.Contains(lower, pat) {
					j.fired = true
					j.onChange(j.id)
					break
				}
			}
		}
	}
	return n, err
}

type joinWatchCloser struct {
	r io.Reader
	c io.Closer
}

func (j *joinWatchCloser) Read(p []byte) (int, error) { return j.r.Read(p) }
func (j *joinWatchCloser) Close() error               { return j.c.Close() }
