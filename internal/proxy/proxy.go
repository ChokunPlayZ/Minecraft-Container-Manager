// Package proxy implements a secure, rate-limited, and cached reverse proxy
// for upstream Minecraft mod and plugin catalog APIs (Modrinth, Hangar, Spiget, CurseForge).
package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	// ErrForbiddenHost is returned when a requested URL is not in the whitelist.
	ErrForbiddenHost = errors.New("host not allowed by proxy")
	// ErrInvalidScheme is returned when a URL does not use HTTPS.
	ErrInvalidScheme = errors.New("only https scheme is permitted")
	// ErrRateLimited is returned when upstream domain is under 429 backoff.
	ErrRateLimited = errors.New("upstream rate limit exceeded")
)

// Allowed upstream hosts.
var allowedHosts = map[string]bool{
	"api.modrinth.com":   true,
	"hangar.papermc.io":  true,
	"api.spiget.org":     true,
	"api.curseforge.com": true,
}

// DomainRateConfig defines pacing and concurrency bounds for an upstream host.
type DomainRateConfig struct {
	MaxConcurrent int
	MinInterval   time.Duration
}

var domainConfigs = map[string]DomainRateConfig{
	"api.spiget.org":     {MaxConcurrent: 1, MinInterval: 350 * time.Millisecond}, // Spiget has strict burst limits
	"api.modrinth.com":   {MaxConcurrent: 3, MinInterval: 100 * time.Millisecond},
	"hangar.papermc.io":  {MaxConcurrent: 3, MinInterval: 100 * time.Millisecond},
	"api.curseforge.com": {MaxConcurrent: 3, MinInterval: 150 * time.Millisecond},
}

var defaultDomainConfig = DomainRateConfig{
	MaxConcurrent: 2,
	MinInterval:   150 * time.Millisecond,
}

type domainState struct {
	mu           sync.Mutex
	sem          chan struct{}
	lastDispatch time.Time
	blockedUntil time.Time
}

type cacheEntry struct {
	StatusCode  int
	ContentType string
	Body        []byte
	CreatedAt   time.Time
	TTL         time.Duration
}

func (e *cacheEntry) isExpired(now time.Time) bool {
	return now.Sub(e.CreatedAt) > e.TTL
}

type singleflightCall struct {
	wg   sync.WaitGroup
	resp *ProxyResponse
	err  error
}

// ProxyResponse holds the proxied response data.
type ProxyResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
	CacheStatus string // HIT, MISS, or STALE
}

// Service manages proxying, caching, deduplication, and rate limiting.
type Service struct {
	client       *http.Client
	domainsMu    sync.Mutex
	domains      map[string]*domainState
	cacheMu      sync.RWMutex
	cache        map[string]*cacheEntry
	flightMu     sync.Mutex
	inFlight     map[string]*singleflightCall
	customClient *http.Client // For test mocking if needed
}

// NewService creates a new initialized proxy Service.
func NewService() *Service {
	return &Service{
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		domains:  make(map[string]*domainState),
		cache:    make(map[string]*cacheEntry),
		inFlight: make(map[string]*singleflightCall),
	}
}

// SetClient overrides the HTTP client, useful for tests.
func (s *Service) SetClient(c *http.Client) {
	s.client = c
}

// Client returns the HTTP client used for requests.
func (s *Service) Client() *http.Client {
	if s == nil || s.client == nil {
		return &http.Client{Timeout: 90 * time.Second}
	}
	return s.client
}

// ClearCache clears all cached responses (useful in tests).
func (s *Service) ClearCache() {
	s.cacheMu.Lock()
	defer s.cacheMu.Unlock()
	s.cache = make(map[string]*cacheEntry)
}

func (s *Service) getDomainState(host string) *domainState {
	s.domainsMu.Lock()
	defer s.domainsMu.Unlock()

	state, ok := s.domains[host]
	if !ok {
		cfg, exists := domainConfigs[host]
		if !exists {
			cfg = defaultDomainConfig
		}
		state = &domainState{
			sem: make(chan struct{}, cfg.MaxConcurrent),
		}
		s.domains[host] = state
	}
	return state
}

func (s *Service) getDomainConfig(host string) DomainRateConfig {
	if cfg, ok := domainConfigs[host]; ok {
		return cfg
	}
	return defaultDomainConfig
}

// IsHostAllowed checks whether the hostname is allowed for proxying.
func IsHostAllowed(host string) bool {
	return allowedHosts[strings.ToLower(host)]
}

// AddAllowedHost adds a host to allowed list (useful for test servers).
func (s *Service) AddAllowedHost(host string) {
	allowedHosts[strings.ToLower(host)] = true
}

func computeCacheKey(method, targetURL string, body []byte) string {
	h := sha256.Sum256(body)
	return fmt.Sprintf("%s:%s:%s", method, targetURL, hex.EncodeToString(h[:8]))
}

// determineTTL returns an appropriate caching TTL based on URL patterns.
func determineTTL(u *url.URL) time.Duration {
	p := u.Path
	switch {
	case strings.Contains(p, "/search") || u.Query().Get("query") != "":
		return 5 * time.Minute
	case strings.Contains(p, "/version_files"):
		return 15 * time.Minute
	case strings.Contains(p, "/version") || strings.Contains(p, "/files"):
		return 10 * time.Minute
	case strings.Contains(p, "/download-url"):
		return 15 * time.Minute
	case strings.Contains(p, "/project/") || strings.Contains(p, "/projects/"):
		return 30 * time.Minute
	default:
		return 5 * time.Minute
	}
}

// Do executes a proxied request, honoring concurrency limits, rate limiting,
// 429 backoff, singleflight deduplication, and in-memory TTL caching.
func (s *Service) Do(ctx context.Context, method, targetURL string, body []byte, headers http.Header, skipCache bool) (*ProxyResponse, error) {
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return nil, fmt.Errorf("invalid url: %w", err)
	}

	if parsedURL.Scheme != "https" {
		// Allow http in local test environments if host is 127.0.0.1 or localhost
		if parsedURL.Scheme != "http" || (!strings.HasPrefix(parsedURL.Host, "127.0.0.1") && !strings.HasPrefix(parsedURL.Host, "localhost")) {
			return nil, ErrInvalidScheme
		}
	}

	host := strings.ToLower(parsedURL.Hostname())
	if !IsHostAllowed(host) {
		return nil, fmt.Errorf("%w: %s", ErrForbiddenHost, host)
	}

	cacheKey := computeCacheKey(method, targetURL, body)

	// 1. Check in-memory cache
	if !skipCache {
		s.cacheMu.RLock()
		entry, found := s.cache[cacheKey]
		if found && !entry.isExpired(time.Now()) {
			s.cacheMu.RUnlock()
			return &ProxyResponse{
				StatusCode:  entry.StatusCode,
				ContentType: entry.ContentType,
				Body:        entry.Body,
				CacheStatus: "HIT",
			}, nil
		}
		s.cacheMu.RUnlock()
	}

	// 2. Singleflight deduplication
	s.flightMu.Lock()
	if call, inProgress := s.inFlight[cacheKey]; inProgress {
		s.flightMu.Unlock()
		call.wg.Wait()
		if call.err != nil {
			return nil, call.err
		}
		// Return copy of deduplicated result
		return &ProxyResponse{
			StatusCode:  call.resp.StatusCode,
			ContentType: call.resp.ContentType,
			Body:        call.resp.Body,
			CacheStatus: "HIT",
		}, nil
	}

	call := &singleflightCall{}
	call.wg.Add(1)
	s.inFlight[cacheKey] = call
	s.flightMu.Unlock()

	defer func() {
		s.flightMu.Lock()
		delete(s.inFlight, cacheKey)
		s.flightMu.Unlock()
		call.wg.Done()
	}()

	// 3. Pacing & Concurrency Queuing
	dState := s.getDomainState(host)
	dConfig := s.getDomainConfig(host)

	// Acquire concurrency semaphore with context cancellation
	select {
	case dState.sem <- struct{}{}:
		defer func() { <-dState.sem }()
	case <-ctx.Done():
		call.err = ctx.Err()
		return nil, ctx.Err()
	}

	// Check if domain is blocked under 429 backoff
	dState.mu.Lock()
	now := time.Now()
	if dState.blockedUntil.After(now) {
		waitDuration := dState.blockedUntil.Sub(now)
		dState.mu.Unlock()

		// If we have any stale cached data, return it to prevent failure
		s.cacheMu.RLock()
		staleEntry, hasStale := s.cache[cacheKey]
		s.cacheMu.RUnlock()
		if hasStale {
			res := &ProxyResponse{
				StatusCode:  staleEntry.StatusCode,
				ContentType: staleEntry.ContentType,
				Body:        staleEntry.Body,
				CacheStatus: "STALE",
			}
			call.resp = res
			return res, nil
		}

		if waitDuration > 10*time.Second {
			call.err = fmt.Errorf("%w: domain %s is cooling down (%v remaining)", ErrRateLimited, host, waitDuration.Round(time.Second))
			return nil, call.err
		}

		// Brief wait if backoff is short
		select {
		case <-time.After(waitDuration):
		case <-ctx.Done():
			call.err = ctx.Err()
			return nil, ctx.Err()
		}
		dState.mu.Lock()
	}

	// Ensure minimum interval between dispatches has elapsed
	elapsed := time.Since(dState.lastDispatch)
	if elapsed < dConfig.MinInterval {
		timeToSleep := dConfig.MinInterval - elapsed
		dState.mu.Unlock()
		select {
		case <-time.After(timeToSleep):
		case <-ctx.Done():
			call.err = ctx.Err()
			return nil, ctx.Err()
		}
		dState.mu.Lock()
	}

	dState.lastDispatch = time.Now()
	dState.mu.Unlock()

	// 4. Execute upstream HTTP request
	var bodyReader io.Reader
	if len(body) > 0 {
		bodyReader = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, targetURL, bodyReader)
	if err != nil {
		call.err = err
		return nil, err
	}

	// Forward allowed headers
	req.Header.Set("User-Agent", "mcm-panel/1.0 (https://github.com/mcm-panel/mcm)")
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	if headers != nil {
		if apiKey := headers.Get("x-api-key"); apiKey != "" {
			req.Header.Set("x-api-key", apiKey)
		}
	}

	resp, err := s.client.Do(req)
	if err != nil {
		call.err = err
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10MB limit
	if err != nil {
		call.err = err
		return nil, err
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/json"
	}

	// 5. Handle HTTP 429 Too Many Requests
	if resp.StatusCode == http.StatusTooManyRequests {
		backoff := 10 * time.Second
		if retryAfterStr := resp.Header.Get("Retry-After"); retryAfterStr != "" {
			if sec, parseErr := strconv.Atoi(retryAfterStr); parseErr == nil && sec > 0 {
				backoff = time.Duration(sec) * time.Second
				if backoff > 60*time.Second {
					backoff = 60 * time.Second
				}
			}
		}

		dState.mu.Lock()
		dState.blockedUntil = time.Now().Add(backoff)
		dState.mu.Unlock()

		// If stale cache exists, serve it
		s.cacheMu.RLock()
		staleEntry, hasStale := s.cache[cacheKey]
		s.cacheMu.RUnlock()
		if hasStale {
			res := &ProxyResponse{
				StatusCode:  staleEntry.StatusCode,
				ContentType: staleEntry.ContentType,
				Body:        staleEntry.Body,
				CacheStatus: "STALE",
			}
			call.resp = res
			return res, nil
		}

		call.err = fmt.Errorf("%w: upstream returned 429 Too Many Requests", ErrRateLimited)
		return nil, call.err
	}

	// 6. Cache successful responses
	if resp.StatusCode == http.StatusOK {
		ttl := determineTTL(parsedURL)
		s.cacheMu.Lock()
		s.cache[cacheKey] = &cacheEntry{
			StatusCode:  resp.StatusCode,
			ContentType: contentType,
			Body:        respBody,
			CreatedAt:   time.Now(),
			TTL:         ttl,
		}
		s.cacheMu.Unlock()
	}

	res := &ProxyResponse{
		StatusCode:  resp.StatusCode,
		ContentType: contentType,
		Body:        respBody,
		CacheStatus: "MISS",
	}
	call.resp = res
	return res, nil
}
