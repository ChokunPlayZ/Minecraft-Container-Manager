package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/dns"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
)

type fakeAPITransport struct {
	mu        sync.Mutex
	requests  []*http.Request
	responses []*http.Response
	index     int
}

func (f *fakeAPITransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, req)
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

func testJSONResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newDNSTestServer(t *testing.T, responses ...*http.Response) (*db.Store, *dns.Service, *servers.Store, http.Handler, string) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "api_dns_test.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	ctx := context.Background()

	// Seed user and session for auth
	users := auth.NewUsers(store.DB)
	user, err := users.Create(ctx, "admin@example.com", "secret-password")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessions := auth.NewManager(store.DB)
	token, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// Seed DNS settings
	_, err = store.DB.ExecContext(ctx, `
		INSERT OR REPLACE INTO settings (key, value) VALUES
			('dns_publish', 'true'),
			('dns_domain', 'example.com'),
			('dns_zone', 'zone-xyz'),
			('dns_api_token', 'token-123'),
			('dns_host', 'node.example.com');
	`)
	if err != nil {
		t.Fatalf("seed settings: %v", err)
	}

	// Seed server
	_, err = store.DB.ExecContext(ctx, `
		INSERT INTO servers (id, name, server_type, version, ram_mb, host_port, created_at, updated_at)
		VALUES ('srv-demo', 'Survival SMP', 'paper', '1.21.4', 4096, 25565, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
	`)
	if err != nil {
		t.Fatalf("seed server: %v", err)
	}

	ft := &fakeAPITransport{responses: responses}
	dnsService := dns.New(store.DB)
	dnsService.SetHTTPClient(&http.Client{Transport: ft})

	dockerMgr, _ := docker.New("unix:///dev/null", "itzg/minecraft-server")
	jarResolver := jars.NewResolver()
	serverStore := servers.NewStore(store, dockerMgr, jarResolver, 25565, 25575, t.TempDir(), "")
	serverStore.SetDNS(dnsService)

	handler := New(Options{
		DB:       store,
		Users:    users,
		Sessions: sessions,
		Servers:  serverStore,
		DNS:      dnsService,
	})

	return store, dnsService, serverStore, handler, token
}

func addAuthAndCSRF(t *testing.T, req *http.Request, token string) {
	t.Helper()
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	csrf := mustCSRF(t)
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
}

func TestHandleListDNS(t *testing.T) {
	_, _, _, handler, token := newDNSTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/dns", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/dns code = %d, want 200. body: %s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Configed bool `json:"configured"`
		Config   struct {
			Domain   string `json:"domain"`
			HasToken bool   `json:"has_token"`
		} `json:"config"`
		Records []any `json:"records"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Configed || resp.Config.Domain != "example.com" || !resp.Config.HasToken {
		t.Errorf("unexpected list response: %#v", resp)
	}
}

func TestHandleTestDNS(t *testing.T) {
	_, _, _, handler, token := newDNSTestServer(t,
		testJSONResponse(200, `{"success":true,"result":{"id":"zone-xyz","name":"example.com","status":"active"}}`),
	)

	body := `{"api_token":"token-123","zone":"zone-xyz","domain":"example.com"}`
	req := httptest.NewRequest(http.MethodPost, "/api/dns/test", bytes.NewBufferString(body))
	addAuthAndCSRF(t, req, token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/dns/test code = %d, want 200. body: %s", rec.Code, rec.Body.String())
	}

	var res struct {
		OK       bool   `json:"ok"`
		ZoneName string `json:"zone_name"`
		Status   string `json:"status"`
		Message  string `json:"message"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
		t.Fatalf("decode test dns resp: %v", err)
	}
	if !res.OK || res.ZoneName != "example.com" || res.Status != "active" {
		t.Errorf("unexpected test dns result: %#v", res)
	}
}

func TestHandlePublishAndGetAndRemoveDNS(t *testing.T) {
	_, _, _, handler, token := newDNSTestServer(t,
		// 1. findSRVRecord -> not found
		testJSONResponse(200, `{"success":true,"result":[]}`),
		// 2. createRecord
		testJSONResponse(200, `{"success":true,"result":{"id":"cf-demo-1"}}`),
		// 3. deleteRecord
		testJSONResponse(200, `{"success":true,"result":null}`),
	)

	// Publish with custom subdomain "smp"
	publishBody := `{"subdomain":"smp","target":"node.example.com","port":25565}`
	postReq := httptest.NewRequest(http.MethodPost, "/api/servers/srv-demo/dns", bytes.NewBufferString(publishBody))
	addAuthAndCSRF(t, postReq, token)
	postRec := httptest.NewRecorder()
	handler.ServeHTTP(postRec, postReq)

	if postRec.Code != http.StatusOK {
		t.Fatalf("POST /api/servers/srv-demo/dns code = %d, want 200. body: %s", postRec.Code, postRec.Body.String())
	}

	var pubResp struct {
		OK          bool   `json:"ok"`
		JoinAddress string `json:"join_address"`
		Record      struct {
			RecordID  string `json:"record_id"`
			Subdomain string `json:"subdomain"`
			Name      string `json:"name"`
		} `json:"record"`
	}
	if err := json.Unmarshal(postRec.Body.Bytes(), &pubResp); err != nil {
		t.Fatalf("decode pubResp: %v", err)
	}
	if !pubResp.OK || pubResp.JoinAddress != "smp.example.com" || pubResp.Record.RecordID != "cf-demo-1" {
		t.Errorf("unexpected publish response: %#v", pubResp)
	}

	// GET server DNS
	getReq := httptest.NewRequest(http.MethodGet, "/api/servers/srv-demo/dns", nil)
	getReq.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	getRec := httptest.NewRecorder()
	handler.ServeHTTP(getRec, getReq)

	if getRec.Code != http.StatusOK {
		t.Fatalf("GET /api/servers/srv-demo/dns code = %d, want 200", getRec.Code)
	}
	var getResp struct {
		JoinAddress string `json:"join_address"`
		Record      struct {
			Subdomain string `json:"subdomain"`
		} `json:"record"`
	}
	if err := json.Unmarshal(getRec.Body.Bytes(), &getResp); err != nil {
		t.Fatalf("decode getResp: %v", err)
	}
	if getResp.JoinAddress != "smp.example.com" || getResp.Record.Subdomain != "smp" {
		t.Errorf("unexpected getResp: %#v", getResp)
	}

	// Remove DNS
	delReq := httptest.NewRequest(http.MethodDelete, "/api/servers/srv-demo/dns", nil)
	addAuthAndCSRF(t, delReq, token)
	delRec := httptest.NewRecorder()
	handler.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("DELETE /api/servers/srv-demo/dns code = %d, want 200", delRec.Code)
	}
}
