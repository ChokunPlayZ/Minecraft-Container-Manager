package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mcm-panel/mcm/internal/proxy"
)

func TestHandleProxyValidation(t *testing.T) {
	px := proxy.NewService()
	s := &Server{proxy: px}

	// 1. Missing url param
	req := httptest.NewRequest(http.MethodGet, "/api/proxy", nil)
	rr := httptest.NewRecorder()
	s.handleProxy(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing url, got %d", rr.Code)
	}

	// 2. Disallowed host
	req = httptest.NewRequest(http.MethodGet, "/api/proxy?url=https://evil.com/malicious", nil)
	rr = httptest.NewRecorder()
	s.handleProxy(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for forbidden host, got %d", rr.Code)
	}
}

func TestHandleProxySuccess(t *testing.T) {
	mockUpstreamBody := `{"hits":[{"title":"Fabric API"}]}`
	mockClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) *http.Response {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(bytes.NewReader([]byte(mockUpstreamBody))),
			}
		}),
	}

	px := proxy.NewService()
	px.SetClient(mockClient)
	s := &Server{proxy: px}

	req := httptest.NewRequest(http.MethodGet, "/api/proxy?url=https://api.modrinth.com/v2/search?query=fabric", nil)
	rr := httptest.NewRecorder()
	s.handleProxy(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("X-MCM-Cache") != "MISS" {
		t.Errorf("expected X-MCM-Cache MISS on first request, got %s", rr.Header().Get("X-MCM-Cache"))
	}

	var res map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	// Second request should hit cache
	req2 := httptest.NewRequest(http.MethodGet, "/api/proxy?url=https://api.modrinth.com/v2/search?query=fabric", nil)
	rr2 := httptest.NewRecorder()
	s.handleProxy(rr2, req2)
	if rr2.Header().Get("X-MCM-Cache") != "HIT" {
		t.Errorf("expected X-MCM-Cache HIT on second request, got %s", rr2.Header().Get("X-MCM-Cache"))
	}
}

type roundTripFunc func(req *http.Request) *http.Response

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req), nil
}
