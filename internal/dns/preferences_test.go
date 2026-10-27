package dns

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/db"
)

func preferenceService(t *testing.T, ft *fakeTransport) (*Service, *db.Store) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "dns.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	_, err = store.DB.Exec(`INSERT INTO servers (id, name, server_type, version, ram_mb, host_port, created_at, updated_at)
		VALUES ('srv-1', 'Original Server', 'paper', '1.21.4', 4096, 25570, '', '');
		INSERT OR REPLACE INTO settings (key, value) VALUES
		('dns_publish','true'), ('dns_domain','example.com'), ('dns_zone','zone-123'), ('dns_api_token','cf-token');`)
	if err != nil {
		t.Fatal(err)
	}
	svc := New(store.DB)
	svc.SetHTTPClient(&http.Client{Transport: ft})
	return svc, store
}

func TestCustomNameSurvivesRemovalRenameAndServiceRestart(t *testing.T) {
	for _, subdomain := range []string{"play", "@", "root"} {
		t.Run(subdomain, func(t *testing.T) {
			ft := &fakeTransport{responses: []*http.Response{
				jsonResponse(200, `{"success":true,"result":[]}`),
				jsonResponse(200, `{"success":true,"result":{"id":"one"}}`),
				jsonResponse(200, `{"success":true,"result":null}`),
				jsonResponse(200, `{"success":true,"result":[]}`),
				jsonResponse(200, `{"success":true,"result":{"id":"two"}}`),
			}}
			svc, store := preferenceService(t, ft)
			ctx := context.Background()
			rec, err := svc.UpsertWithOptions(ctx, "srv-1", PublishOptions{Subdomain: subdomain})
			if err != nil {
				t.Fatal(err)
			}
			want := rec.Subdomain
			if err := svc.Remove(ctx, "srv-1"); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.GetRecord(ctx, "srv-1"); !errors.Is(err, sql.ErrNoRows) {
				t.Fatalf("record still tracked: %v", err)
			}
			if saved, err := svc.GetSubdomain(ctx, "srv-1"); err != nil || saved != want {
				t.Fatalf("saved preference=%q err=%v", saved, err)
			}
			if _, err := store.DB.Exec(`UPDATE servers SET name = 'Renamed Server' WHERE id = 'srv-1'`); err != nil {
				t.Fatal(err)
			}
			// A fresh service instance simulates an application restart.
			restarted := New(store.DB)
			restarted.SetHTTPClient(&http.Client{Transport: ft})
			if err := restarted.Upsert(ctx, "srv-1", "", 25571); err != nil {
				t.Fatal(err)
			}
			after, err := restarted.GetRecord(ctx, "srv-1")
			if err != nil || after.Subdomain != want || after.Name != rec.Name || after.Port != 25571 {
				t.Fatalf("custom address changed after restart: %#v, %v", after, err)
			}
			if _, err := store.DB.Exec(`DELETE FROM servers WHERE id = 'srv-1'`); err != nil {
				t.Fatal(err)
			}
			if saved, err := restarted.GetSubdomain(ctx, "srv-1"); err != nil || saved != "" {
				t.Fatalf("preference survived server deletion: %q, %v", saved, err)
			}
		})
	}
}

func TestCleanupUsesOriginalProviderEvenWhenPublishingDisabled(t *testing.T) {
	ft := &fakeTransport{responses: []*http.Response{
		jsonResponse(200, `{"status":"SUCCESS","records":[]}`),
		jsonResponse(200, `{"status":"SUCCESS","id":"123"}`),
		jsonResponse(200, `{"status":"SUCCESS"}`),
	}}
	svc, store := preferenceService(t, ft)
	_, err := store.DB.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES
		('dns_provider','porkbun'), ('dns_porkbun_api_token','pk-test'), ('dns_porkbun_api_secret','sk-test');`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpsertWithOptions(context.Background(), "srv-1", PublishOptions{Subdomain: "play"}); err != nil {
		t.Fatal(err)
	}
	_, err = store.DB.Exec(`UPDATE settings SET value='cloudflare' WHERE key='dns_provider'; UPDATE settings SET value='false' WHERE key='dns_publish';`)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Remove(context.Background(), "srv-1"); err != nil {
		t.Fatal(err)
	}
	last := ft.requests[len(ft.requests)-1]
	if last.req.URL.Host != "api.porkbun.com" || !strings.Contains(string(last.body), `"apikey":"pk-test"`) {
		t.Fatalf("cleanup used wrong provider: %s %s", last.req.URL, last.body)
	}
}

func TestFailedCleanupKeepsTrackingAndSavedName(t *testing.T) {
	ft := &fakeTransport{responses: []*http.Response{
		jsonResponse(200, `{"success":true,"result":[]}`),
		jsonResponse(200, `{"success":true,"result":{"id":"one"}}`),
		jsonResponse(403, `{"success":false,"errors":[{"message":"permission denied"}]}`),
	}}
	svc, _ := preferenceService(t, ft)
	ctx := context.Background()
	if _, err := svc.UpsertWithOptions(ctx, "srv-1", PublishOptions{Subdomain: "play"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.Remove(ctx, "srv-1"); err == nil {
		t.Fatal("expected deletion failure")
	}
	if rec, err := svc.GetRecord(ctx, "srv-1"); err != nil || rec.RecordID != "one" {
		t.Fatalf("lost tracking after failed deletion: %#v %v", rec, err)
	}
}

func TestSwitchProviderRemovesOriginalRecordAndKeepsName(t *testing.T) {
	ft := &fakeTransport{responses: []*http.Response{
		jsonResponse(200, `{"success":true,"result":[]}`),
		jsonResponse(200, `{"success":true,"result":{"id":"cf-one"}}`),
		jsonResponse(200, `{"success":true,"result":null}`),
		jsonResponse(200, `{"domain_records":[]}`),
		jsonResponse(201, `{"domain_record":{"id":42}}`),
	}}
	svc, store := preferenceService(t, ft)
	ctx := context.Background()
	if _, err := svc.UpsertWithOptions(ctx, "srv-1", PublishOptions{Subdomain: "play"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DB.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES ('dns_provider','digitalocean'), ('dns_digitalocean_api_token','do-token');`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Upsert(ctx, "srv-1", "", 25571); err != nil {
		t.Fatal(err)
	}
	rec, err := svc.GetRecord(ctx, "srv-1")
	if err != nil || rec.Provider != "digitalocean" || rec.RecordID != "42" || rec.Subdomain != "play" {
		t.Fatalf("provider switch: %#v %v", rec, err)
	}
	if req := ft.requests[2].req; req.Method != "DELETE" || req.URL.Host != "api.cloudflare.com" || req.Header.Get("Authorization") != "Bearer cf-token" {
		t.Fatalf("old record not removed with original credentials: %v", req)
	}
	if req := ft.requests[4].req; req.URL.Host != "api.digitalocean.com" || req.Header.Get("Authorization") != "Bearer do-token" {
		t.Fatalf("wrong destination credentials: %v", req)
	}
}

func TestVerifyUsesSavedZoneForNestedBaseDomain(t *testing.T) {
	ft := &fakeTransport{responses: []*http.Response{jsonResponse(200, `{"status":"SUCCESS","records":[]}`)}}
	svc, store := preferenceService(t, ft)
	_, err := store.DB.Exec(`INSERT OR REPLACE INTO settings (key, value) VALUES
		('dns_provider','porkbun'), ('dns_domain','games.example.com'), ('dns_porkbun_zone','example.com'),
		('dns_porkbun_api_token','pk-test'), ('dns_porkbun_api_secret','sk-test');`)
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.VerifyWithOptions(context.Background(), VerifyOptions{})
	if err != nil || !result.OK || !strings.HasSuffix(ft.requests[0].req.URL.Path, "/dns/retrieve/example.com") {
		t.Fatalf("saved zone ignored: %#v %v", result, err)
	}
}

func TestProviderConfigurationAndVerification(t *testing.T) {
	for _, name := range []string{"cloudflare", "porkbun", "digitalocean", "route53"} {
		t.Run(name, func(t *testing.T) {
			ft := &fakeTransport{}
			svc, store := preferenceService(t, ft)
			_, err := store.DB.Exec(`UPDATE settings SET value='false' WHERE key='dns_publish'`)
			if err != nil {
				t.Fatal(err)
			}
			body := `{"success":true,"result":{"name":"example.com","status":"active"}}`
			switch name {
			case "porkbun":
				body = `{"status":"SUCCESS","records":[]}`
			case "digitalocean":
				body = `{"domain":{"name":"example.com"}}`
			case "route53":
				body = `<GetHostedZoneResponse><HostedZone><Name>example.com.</Name><Config><PrivateZone>false</PrivateZone></Config></HostedZone></GetHostedZoneResponse>`
			}
			ft.responses = []*http.Response{jsonResponse(200, body)}
			result, err := svc.VerifyWithOptions(context.Background(), VerifyOptions{Provider: name, APIToken: "new-key", APISecret: "secret", Domain: "games.example.com", Zone: "example.com"})
			if err != nil || !result.OK {
				t.Fatalf("verify while disabled: %#v %v", result, err)
			}
			if strings.Contains(result.Message, "new-key") || strings.Contains(result.Message, "secret") {
				t.Fatal("verification leaked credentials")
			}
		})
	}
	if cfg := buildConfig(map[string]string{KeyProvider: "porkbun", KeyDomain: "example.com", KeyAPIToken: "cloudflare-token"}); cfg.hasCredentials() {
		t.Fatal("Porkbun reused Cloudflare credentials")
	}
	for name, ttl := range map[string]int{"porkbun": 600, "digitalocean": 30} {
		cfg := buildConfig(map[string]string{KeyProvider: name, KeyTTL: "1"})
		if cfg.TTL != ttl {
			t.Fatalf("%s TTL=%d want %d", name, cfg.TTL, ttl)
		}
	}
}
