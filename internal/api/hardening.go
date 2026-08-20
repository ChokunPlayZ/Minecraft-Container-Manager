package api

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/mcm-panel/mcm/internal/config"
)

// loginDefaults returns the login limiter parameters, honoring runtime
// configuration or falling back to sensible defaults when config is absent.
func loginDefaults(cfg *config.Config) (int, time.Duration, time.Duration) {
	max := 5
	window := time.Minute
	lockout := 15 * time.Minute
	if cfg != nil {
		if cfg.LoginMaxAttempts > 0 {
			max = cfg.LoginMaxAttempts
		}
		if cfg.LoginLockout > 0 {
			lockout = cfg.LoginLockout
		}
	}
	return max, window, lockout
}

// clientIP returns the caller's IP address, stripping any port suffix.
func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return host
}

// ipLimiter is a sliding-window per-client request limiter used by the general
// rate-limiting middleware. It is safe for concurrent use.
type ipLimiter struct {
	mu     sync.Mutex
	window map[string][]time.Time
	max    int
	win    time.Duration
}

func newIPLimiter(max int, win time.Duration) *ipLimiter {
	return &ipLimiter{
		window: make(map[string][]time.Time),
		max:    max,
		win:    win,
	}
}

// allow reports whether the client may proceed, recording the request when it
// is within the limit.
func (l *ipLimiter) allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	cutoff := now.Add(-l.win)
	kept := l.window[key][:0]
	for _, t := range l.window[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	l.window[key] = kept

	if len(l.window[key]) >= l.max {
		return false
	}
	l.window[key] = append(l.window[key], now)
	return true
}

// statusRecorder captures the response status so middleware can log it.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (r *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := r.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (r *statusRecorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

// logRequests is a structured-request-logging middleware. It records method,
// path, status, and duration for every request.
func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		s.logger.Printf("method=%s path=%s status=%d duration=%s remote=%s",
			r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond), s.clientIP(r))
	})
}

// rateLimit is a general per-IP limiter applied to state-changing requests.
func (s *Server) rateLimit(next http.Handler) http.Handler {
	limit := 100
	win := time.Minute
	if s.cfg != nil {
		if s.cfg.RateLimitMax > 0 {
			limit = s.cfg.RateLimitMax
		}
		if s.cfg.RateLimitWindow > 0 {
			win = s.cfg.RateLimitWindow
		}
	}
	lim := &ipLimiter{window: make(map[string][]time.Time), max: limit, win: win}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}
		if !lim.allow(s.clientIP(r), time.Now()) {
			writeError(w, http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	checks := map[string]string{}
	ready := true

	if s.db != nil && s.db.DB != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := s.db.DB.PingContext(ctx); err != nil {
			checks["db"] = "unreachable"
			ready = false
		} else {
			checks["db"] = "ok"
		}
	} else {
		checks["db"] = "unavailable"
		ready = false
	}

	if s.servers != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		dockerStatus := s.servers.DockerStatus(ctx)
		if !dockerStatus.Reachable {
			checks["docker"] = "unreachable"
			ready = false
		} else {
			checks["docker"] = "ok"
			if !dockerStatus.ImageReady {
				checks["docker_image"] = "missing"
				ready = false
			} else {
				checks["docker_image"] = "ok"
			}
		}
	} else {
		checks["docker"] = "unavailable"
		ready = false
	}

	status := http.StatusOK
	state := "ready"
	if !ready {
		status = http.StatusServiceUnavailable
		state = "not_ready"
	}
	writeJSON(w, status, map[string]any{"status": state, "checks": checks})
}
