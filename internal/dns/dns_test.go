package dns

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestSafeLabel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "abc-123", want: "abc-123"},
		{in: "AbC-123", want: "abc-123"},
		{in: "a_b.c!", want: "abc"},
		{in: "", want: "server"},
		{in: "!!!", want: "server"},
	}
	for _, tc := range tests {
		if got := safeLabel(tc.in); got != tc.want {
			t.Errorf("safeLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestConfigTarget(t *testing.T) {
	cfg := &config{Host: "", Domain: "play.example.com"}
	if got := cfg.Target(""); got != "play.example.com" {
		t.Errorf("Target(empty) = %q, want %q", got, "play.example.com")
	}
	if got := cfg.Target("server-host"); got != "server-host" {
		t.Errorf("Target(server-host) = %q, want server-host", got)
	}
	cfg.Host = "fixed.example.com"
	if got := cfg.Target("server-host"); got != "fixed.example.com" {
		t.Errorf("Target with host set = %q, want fixed.example.com", got)
	}
}

func TestConfigRecordName(t *testing.T) {
	cfg := &config{
		Service: "_minecraft",
		Proto:   "_tcp",
		Domain:  "example.com",
	}
	if got := cfg.recordName("abc123"); got != "_minecraft._tcp.abc123.example.com" {
		t.Errorf("recordName = %q", got)
	}
}

func TestBuildConfigDefaults(t *testing.T) {
	cfg := buildConfig(map[string]string{
		KeyPublish: "false",
		KeyTTL:     "",
		KeyService: "",
		KeyProto:   "",
	})
	if cfg.Publish {
		t.Error("Publish should default to false")
	}
	if cfg.Service != defaultService {
		t.Errorf("Service = %q, want %q", cfg.Service, defaultService)
	}
	if cfg.Proto != defaultProto {
		t.Errorf("Proto = %q, want %q", cfg.Proto, defaultProto)
	}
	if cfg.TTL != defaultTTL {
		t.Errorf("TTL = %d, want %d", cfg.TTL, defaultTTL)
	}
}

// fakeTransport responds to requests without binding any network socket.
type fakeTransport struct {
	mu        sync.Mutex
	requests  []capturedRequest
	responses []*http.Response
	index     int
}

type capturedRequest struct {
	req  *http.Request
	body []byte
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if req.Body != nil {
		body, _ := io.ReadAll(req.Body)
		req.Body.Close()
		f.requests = append(f.requests, capturedRequest{req: req, body: body})
	} else {
		f.requests = append(f.requests, capturedRequest{req: req})
	}
	var resp *http.Response
	if f.index < len(f.responses) {
		resp = f.responses[f.index]
		f.index++
	} else {
		resp = &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"success":true,"result":null}`))}
	}
	resp.Request = req
	return resp, nil
}

func jsonResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func TestCFClientLifecycle(t *testing.T) {
	ft := &fakeTransport{
		responses: []*http.Response{
			jsonResponse(200, `{"success":true,"result":{"id":"record-1"}}`),
			jsonResponse(200, `{"success":true,"result":null}`),
			jsonResponse(200, `{"success":true,"result":null}`),
		},
	}
	hc := &http.Client{Transport: ft}
	c := newCFClient("token123", "zone-abc", hc, "https://fake.invalid")

	ctx := context.Background()
	id, err := c.createRecord(ctx, "x.example.com", "target.example.com", 25565, 120, 0, 5)
	if err != nil {
		t.Fatalf("createRecord: %v", err)
	}
	if id != "record-1" {
		t.Errorf("createRecord id = %q, want record-1", id)
	}

	if err := c.updateRecord(ctx, "record-1", "x.example.com", "target.example.com", 25565, 120, 0, 5); err != nil {
		t.Fatalf("updateRecord: %v", err)
	}
	if err := c.deleteRecord(ctx, "record-1"); err != nil {
		t.Fatalf("deleteRecord: %v", err)
	}

	ft.mu.Lock()
	defer ft.mu.Unlock()
	if len(ft.requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(ft.requests))
	}
	postReq := ft.requests[0].req
	if postReq.Method != http.MethodPost || !strings.HasSuffix(postReq.URL.Path, "/zones/zone-abc/dns_records") {
		t.Errorf("createRecord hit %s %s", postReq.Method, postReq.URL.Path)
	}
	var body map[string]any
	if err := json.Unmarshal(ft.requests[0].body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}
	data, _ := body["data"].(map[string]any)
	if body["type"] != "SRV" || data["port"] != float64(25565) || data["weight"] != float64(5) {
		t.Errorf("createRecord body = %#v", body)
	}
	if h := postReq.Header.Get("Authorization"); h != "Bearer token123" {
		t.Errorf("Authorization = %q, want Bearer token123", h)
	}

	put := ft.requests[1].req
	if put.Method != http.MethodPut || !strings.HasSuffix(put.URL.Path, "/dns_records/record-1") {
		t.Errorf("updateRecord hit %s %s", put.Method, put.URL.Path)
	}
	del := ft.requests[2].req
	if del.Method != http.MethodDelete || !strings.HasSuffix(del.URL.Path, "/dns_records/record-1") {
		t.Errorf("deleteRecord hit %s %s", del.Method, del.URL.Path)
	}
}

func TestCFClientErrorEnvelope(t *testing.T) {
	ft := &fakeTransport{
		responses: []*http.Response{
			jsonResponse(400, `{"success":false,"result":null,"errors":[{"code":81044,"message":"permission denied"}]}`),
		},
	}
	c := newCFClient("token123", "zone-abc", &http.Client{Transport: ft}, "https://fake.invalid")
	_, err := c.createRecord(context.Background(), "x.example.com", "t", 25565, 120, 0, 5)
	if err == nil {
		t.Fatal("expected error on API failure")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q, want it to mention permission denied", err.Error())
	}
}
