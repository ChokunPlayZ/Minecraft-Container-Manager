package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProxySSRFProtection(t *testing.T) {
	svc := NewService()

	// 1. Host not allowed
	_, err := svc.Do(context.Background(), http.MethodGet, "https://evil.com/api", nil, nil, false)
	if err == nil {
		t.Fatal("expected error for non-whitelisted host, got nil")
	}

	// 2. Non-HTTPS disallowed
	_, err = svc.Do(context.Background(), http.MethodGet, "http://api.modrinth.com/v2/search", nil, nil, false)
	if err == nil {
		t.Fatal("expected error for non-HTTPS url, got nil")
	}
}

func TestProxyCachingAndSingleflight(t *testing.T) {
	var hitCount int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hitCount, 1)
		time.Sleep(50 * time.Millisecond) // simulate network latency
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":"ok"}`))
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	svc := NewService()
	svc.AddAllowedHost(u.Hostname())
	svc.SetClient(ts.Client())

	ctx := context.Background()

	// 1. Singleflight test: dispatch 5 concurrent requests for identical URL
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := svc.Do(ctx, http.MethodGet, ts.URL+"/search?query=test", nil, nil, false)
			if err != nil {
				t.Errorf("request failed: %v", err)
				return
			}
			if string(resp.Body) != `{"result":"ok"}` {
				t.Errorf("unexpected body: %s", resp.Body)
			}
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&hitCount); got != 1 {
		t.Fatalf("expected singleflight to coalesce requests to 1 hit, got %d", got)
	}

	// 2. Subsequent request should HIT cache immediately
	resp, err := svc.Do(ctx, http.MethodGet, ts.URL+"/search?query=test", nil, nil, false)
	if err != nil {
		t.Fatalf("subsequent call failed: %v", err)
	}
	if resp.CacheStatus != "HIT" {
		t.Errorf("expected CacheStatus HIT, got %s", resp.CacheStatus)
	}
	if got := atomic.LoadInt32(&hitCount); got != 1 {
		t.Fatalf("expected hitCount to remain 1 after cache hit, got %d", got)
	}

	// 3. SkipCache should bypass and fetch fresh
	resp, err = svc.Do(ctx, http.MethodGet, ts.URL+"/search?query=test", nil, nil, true)
	if err != nil {
		t.Fatalf("skipCache call failed: %v", err)
	}
	if resp.CacheStatus != "MISS" {
		t.Errorf("expected CacheStatus MISS, got %s", resp.CacheStatus)
	}
	if got := atomic.LoadInt32(&hitCount); got != 2 {
		t.Fatalf("expected hitCount to be 2 after skipCache, got %d", got)
	}
}

func TestProxyRateLimitAndStaleCache(t *testing.T) {
	var statusCode int32 = http.StatusOK
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		status := int(atomic.LoadInt32(&statusCode))
		if status == http.StatusTooManyRequests {
			w.Header().Set("Retry-After", "1")
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"status":"response"}`))
	}))
	defer ts.Close()

	u, _ := url.Parse(ts.URL)
	svc := NewService()
	svc.AddAllowedHost(u.Hostname())
	svc.SetClient(ts.Client())

	ctx := context.Background()

	// Initial request populates cache
	resp, err := svc.Do(ctx, http.MethodGet, ts.URL+"/projects/test", nil, nil, false)
	if err != nil {
		t.Fatalf("initial request failed: %v", err)
	}
	if resp.CacheStatus != "MISS" {
		t.Fatalf("expected MISS, got %s", resp.CacheStatus)
	}

	// Now simulate upstream returning 429
	atomic.StoreInt32(&statusCode, http.StatusTooManyRequests)

	// Since we already have a cached version, on 429 with skipCache or after expiry it serves STALE cache
	resp, err = svc.Do(ctx, http.MethodGet, ts.URL+"/projects/test", nil, nil, true)
	if err != nil {
		t.Fatalf("expected stale fallback on 429, got error: %v", err)
	}
	if resp.CacheStatus != "STALE" {
		t.Errorf("expected STALE cache status, got %s", resp.CacheStatus)
	}
}
