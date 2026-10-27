package notifications

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mcm-panel/mcm/internal/db"
)

const testWebhook = "https://discord.com/api/webhooks/123456/secret-token"

func newTestService(t *testing.T) *Service {
	t.Helper()
	h, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { h.Close() })
	return New(h.DB, log.New(io.Discard, "", 0))
}

type transportFunc func(*http.Request) (*http.Response, error)

func (f transportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}
}

func TestSettingsPersistenceAndSecret(t *testing.T) {
	s := newTestService(t)
	ctx := context.Background()
	defaults, err := s.Settings(ctx)
	if err != nil || defaults.Enabled || defaults.Configured || len(defaults.Events) != 6 {
		t.Fatalf("defaults: %+v %v", defaults, err)
	}
	u := testWebhook
	settings, err := s.Save(ctx, Update{Enabled: true, WebhookURL: &u, Events: []string{ServerCrashed, ServerCrashed}})
	if err != nil {
		t.Fatal(err)
	}
	if !settings.Configured || !settings.Enabled || len(settings.Events) != 1 {
		t.Fatalf("settings: %+v", settings)
	}
	raw, _ := json.Marshal(settings)
	if strings.Contains(string(raw), "secret-token") || strings.Contains(string(raw), "webhook_url") {
		t.Fatalf("secret exposed: %s", raw)
	}
	// Saving other choices keeps the stored secret; a new service sees it.
	_, err = s.Save(ctx, Update{Events: []string{ServerStarted}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(s.db, nil).config(ctx)
	if err != nil || c.WebhookURL != testWebhook || c.Enabled || len(c.Events) != 1 || c.Events[0] != ServerStarted {
		t.Fatalf("persisted: %+v %v", c, err)
	}
	empty := ""
	if _, err := s.Save(ctx, Update{Enabled: true, WebhookURL: &empty}); err == nil {
		t.Fatal("enabled without webhook")
	}
	if _, err := s.Save(ctx, Update{Events: []string{"unknown"}}); err == nil {
		t.Fatal("unknown event accepted")
	}
	if _, err := s.Save(ctx, Update{WebhookURL: &empty, Events: []string{}}); err != nil {
		t.Fatal(err)
	}
	settings, _ = s.Settings(ctx)
	if settings.Configured || settings.Enabled || settings.Events == nil {
		t.Fatalf("cleared: %+v", settings)
	}
}

func TestValidateURL(t *testing.T) {
	for _, raw := range []string{testWebhook, "https://discord.com/api/v10/webhooks/123/token_abc-123?thread_id=1234", "https://discordapp.com/api/webhooks/1/token?wait=false"} {
		if err := ValidateURL(raw); err != nil {
			t.Errorf("valid %q: %v", raw, err)
		}
	}
	for _, raw := range []string{"", "http://discord.com/api/webhooks/1/token", "https://localhost/api/webhooks/1/token", "https://discord.com.evil.test/api/webhooks/1/token", "https://discord.com:443/api/webhooks/1/token", "https://user@discord.com/api/webhooks/1/token", "https://discord.com/api/webhooks/1/token/slack", "https://discord.com/api/webhooks/1/token#fragment", "https://discord.com/api/webhooks/1/token?thread_id=bad", "https://discord.com/api/webhooks/1/token?redirect=https://example.com"} {
		if err := ValidateURL(raw); err == nil {
			t.Errorf("accepted %q", raw)
		}
	}
}

func TestPayloadAndRateLimit(t *testing.T) {
	s := newTestService(t)
	attempts := 0
	s.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		attempts++
		if r.Method != http.MethodPost || r.URL.Host != "discord.com" || r.URL.Query().Get("wait") != "true" || r.URL.Query().Get("thread_id") != "987" {
			t.Errorf("request: %s %s", r.Method, r.URL)
		}
		var payload struct {
			AllowedMentions struct {
				Parse []string `json:"parse"`
			} `json:"allowed_mentions"`
			Embeds []struct {
				Title, Description, Timestamp string
				Fields                        []struct{ Name, Value string }
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.AllowedMentions.Parse == nil || len(payload.AllowedMentions.Parse) != 0 {
			t.Fatal("mentions not disabled")
		}
		if len(payload.Embeds) != 1 || payload.Embeds[0].Title != "Server crashed" || len([]rune(payload.Embeds[0].Description)) > 4096 || len([]rune(payload.Embeds[0].Fields[0].Value)) > 1024 {
			t.Fatalf("invalid embed: %+v", payload)
		}
		if attempts == 1 {
			return response(429, `{"retry_after":0.001}`), nil
		}
		return response(200, `{}`), nil
	})
	err := s.send(context.Background(), testWebhook+"?thread_id=987", Event{Kind: ServerCrashed, ServerID: "server-1", ServerName: strings.Repeat("界", 1100), Message: strings.Repeat("界", 5000), Time: time.Now()})
	if err != nil || attempts != 2 {
		t.Fatalf("retry: %d %v", attempts, err)
	}
}

func TestDeliveryErrorsDoNotLeakSecret(t *testing.T) {
	s := newTestService(t)
	for _, status := range []int{302, 401, 404, 429, 500} {
		s.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return response(status, testWebhook), nil })
		err := s.Test(context.Background(), ptr(testWebhook))
		if err == nil || strings.Contains(err.Error(), "secret-token") {
			t.Fatalf("status %d: %v", status, err)
		}
	}
	s.client.Transport = transportFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New(testWebhook) })
	if err := s.Test(context.Background(), ptr(testWebhook)); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("transport: %v", err)
	}
}

func ptr(s string) *string { return &s }

func TestWorkerFiltersEventsAndStops(t *testing.T) {
	s := newTestService(t)
	_, err := s.Save(context.Background(), Update{Enabled: true, WebhookURL: ptr(testWebhook), Events: []string{ServerCrashed}})
	if err != nil {
		t.Fatal(err)
	}
	sent := make(chan string, 10)
	s.client.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		sent <- string(body)
		return response(204, ""), nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	s.Notify(Event{Kind: ServerStarted})
	s.Notify(Event{Kind: ServerCrashed, Message: "crash"})
	select {
	case body := <-sent:
		if !strings.Contains(body, "Server crashed") {
			t.Fatal(body)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no crash delivered")
	}
	_, err = s.Save(context.Background(), Update{Enabled: false, Events: []string{ServerCrashed}})
	if err != nil {
		t.Fatal(err)
	}
	// Test messages work even when automatic notifications are disabled.
	if err := s.Test(context.Background(), nil); err != nil {
		t.Fatal(err)
	}
	<-sent
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("worker did not stop")
	}
	select {
	case body := <-sent:
		t.Fatalf("extra notification: %s", body)
	default:
	}
}
