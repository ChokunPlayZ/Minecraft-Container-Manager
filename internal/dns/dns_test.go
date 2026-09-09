package dns

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
)

func TestSafeLabel(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "abc-123", want: "abc-123"},
		{in: "AbC-123", want: "abc-123"},
		{in: "a_b.c!", want: "a-bc"},
		{in: "Vanilla SMP Server", want: "vanilla-smp-server"},
		{in: "", want: "server"},
		{in: "!!!", want: "server"},
		{in: "--hyphens--", want: "hyphens"},
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
		t.Errorf("recordName(abc123) = %q", got)
	}
	// Apex / root domain
	if got := cfg.recordName(""); got != "_minecraft._tcp.example.com" {
		t.Errorf("recordName(empty) = %q, want _minecraft._tcp.example.com", got)
	}
	if got := cfg.recordName("@"); got != "_minecraft._tcp.example.com" {
		t.Errorf("recordName(@) = %q, want _minecraft._tcp.example.com", got)
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
	if cfg.Priority != defaultPriority {
		t.Errorf("Priority = %d, want %d", cfg.Priority, defaultPriority)
	}
	if cfg.Weight != defaultWeight {
		t.Errorf("Weight = %d, want %d", cfg.Weight, defaultWeight)
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
			jsonResponse(200, `{"success":true,"result":[{"id":"record-1"}]}`),
			jsonResponse(200, `{"success":true,"result":{"id":"zone-abc","name":"example.com","status":"active"}}`),
			jsonResponse(200, `{"success":true,"result":null}`),
		},
	}
	hc := &http.Client{Transport: ft}
	c := newCFClient("token123", "zone-abc", hc, "https://fake.invalid")

	ctx := context.Background()
	id, err := c.createRecord(ctx, "_minecraft", "_tcp", "x.example.com", "_minecraft._tcp.x.example.com", "target.example.com", 25565, 120, 0, 5)
	if err != nil {
		t.Fatalf("createRecord: %v", err)
	}
	if id != "record-1" {
		t.Errorf("createRecord id = %q, want record-1", id)
	}

	if err := c.updateRecord(ctx, "record-1", "_minecraft", "_tcp", "x.example.com", "_minecraft._tcp.x.example.com", "target.example.com", 25565, 120, 0, 5); err != nil {
		t.Fatalf("updateRecord: %v", err)
	}

	foundID, err := c.findSRVRecord(ctx, "_minecraft._tcp.x.example.com")
	if err != nil {
		t.Fatalf("findSRVRecord: %v", err)
	}
	if foundID != "record-1" {
		t.Errorf("findSRVRecord id = %q, want record-1", foundID)
	}

	zoneInfo, err := c.verifyZone(ctx)
	if err != nil {
		t.Fatalf("verifyZone: %v", err)
	}
	if zoneInfo.Name != "example.com" {
		t.Errorf("zoneInfo name = %q, want example.com", zoneInfo.Name)
	}

	if err := c.deleteRecord(ctx, "record-1"); err != nil {
		t.Fatalf("deleteRecord: %v", err)
	}

	ft.mu.Lock()
	defer ft.mu.Unlock()
	if len(ft.requests) != 5 {
		t.Fatalf("expected 5 requests, got %d", len(ft.requests))
	}

	postReq := ft.requests[0].req
	if postReq.Method != http.MethodPost || !strings.HasSuffix(postReq.URL.Path, "/zones/zone-abc/dns_records") {
		t.Errorf("createRecord hit %s %s", postReq.Method, postReq.URL.Path)
	}
	var body map[string]any
	if err := json.Unmarshal(ft.requests[0].body, &body); err != nil {
		t.Fatalf("decode request body: %v", err)
	}

	// Verify Cloudflare v4 payload rules:
	// 1. NO "content" field!
	if _, hasContent := body["content"]; hasContent {
		t.Errorf("createRecord payload must NOT contain content field: %#v", body)
	}
	// 2. "data" must have required fields
	data, _ := body["data"].(map[string]any)
	if data == nil {
		t.Fatalf("data field missing in payload: %#v", body)
	}
	if data["service"] != "_minecraft" || data["proto"] != "_tcp" || data["name"] != "x.example.com" {
		t.Errorf("data service/proto/name incorrect: %#v", data)
	}
	if data["port"] != float64(25565) || data["weight"] != float64(5) || data["priority"] != float64(0) {
		t.Errorf("data port/weight/priority incorrect: %#v", data)
	}
	if data["target"] != "target.example.com" {
		t.Errorf("data target = %v, want target.example.com", data["target"])
	}

	put := ft.requests[1].req
	if put.Method != http.MethodPut || !strings.HasSuffix(put.URL.Path, "/dns_records/record-1") {
		t.Errorf("updateRecord hit %s %s", put.Method, put.URL.Path)
	}
}

func TestCFClientErrorEnvelope(t *testing.T) {
	ft := &fakeTransport{
		responses: []*http.Response{
			jsonResponse(400, `{"success":false,"result":null,"errors":[{"code":81044,"message":"permission denied"}]}`),
		},
	}
	c := newCFClient("token123", "zone-abc", &http.Client{Transport: ft}, "https://fake.invalid")
	_, err := c.createRecord(context.Background(), "_minecraft", "_tcp", "x", "x.example.com", "t", 25565, 120, 0, 5)
	if err == nil {
		t.Fatal("expected error on API failure")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Errorf("error = %q, want it to mention permission denied", err.Error())
	}
}

func TestServiceDatabaseOperations(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "dns_test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// Seed server and settings
	_, err = store.DB.ExecContext(ctx, `
		INSERT INTO servers (id, name, server_type, version, ram_mb, host_port, created_at, updated_at)
		VALUES ('srv-1', 'Mega Survival', 'paper', '1.21.4', 4096, 25570, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
		INSERT OR REPLACE INTO settings (key, value) VALUES
			('dns_publish', 'true'),
			('dns_domain', 'example.com'),
			('dns_zone', 'zone-123'),
			('dns_api_token', 'token-abc'),
			('dns_host', 'node1.example.com');
	`)
	if err != nil {
		t.Fatalf("seed db: %v", err)
	}

	ft := &fakeTransport{
		responses: []*http.Response{
			jsonResponse(200, `{"success":true,"result":[]}`),
			jsonResponse(200, `{"success":true,"result":{"id":"cf-rec-1"}}`),
			jsonResponse(200, `{"success":true,"result":null}`),
		},
	}
	svc := New(store.DB)
	svc.http = &http.Client{Transport: ft}

	// 1. Get safe config
	cfgInfo, err := svc.GetConfig(ctx)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if !cfgInfo.Publish || !cfgInfo.HasToken || cfgInfo.Domain != "example.com" {
		t.Errorf("unexpected config info: %#v", cfgInfo)
	}

	// 2. Upsert with explicit subdomain
	rec, err := svc.UpsertWithOptions(ctx, "srv-1", PublishOptions{
		Subdomain: "play",
		Port:      25570,
	})
	if err != nil {
		t.Fatalf("UpsertWithOptions: %v", err)
	}
	if rec.Subdomain != "play" || rec.Name != "_minecraft._tcp.play.example.com" || rec.RecordID != "cf-rec-1" {
		t.Errorf("unexpected record: %#v", rec)
	}

	// 3. GetRecord
	fetched, err := svc.GetRecord(ctx, "srv-1")
	if err != nil {
		t.Fatalf("GetRecord: %v", err)
	}
	if fetched.Subdomain != "play" || fetched.Port != 25570 {
		t.Errorf("unexpected fetched record: %#v", fetched)
	}

	// 4. List records
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ServerID != "srv-1" {
		t.Errorf("unexpected list: %#v", list)
	}

	// 5. Remove record
	if err := svc.Remove(ctx, "srv-1"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	listAfter, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List after remove: %v", err)
	}
	if len(listAfter) != 0 {
		t.Errorf("expected 0 records after remove, got %d", len(listAfter))
	}
}
