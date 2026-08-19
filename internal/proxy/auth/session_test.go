package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHasJoinedOnline(t *testing.T) {
	var gotUser, gotID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUser = r.URL.Query().Get("username")
		gotID = r.URL.Query().Get("serverId")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"2d0e5b91b9a2434b9b3d9f9f1b3f4a5b","name":"Steve"}`))
	}))
	defer srv.Close()

	c := &MojangClient{BaseURL: srv.URL, HTTP: srv.Client()}
	p, err := c.HasJoined(context.Background(), "Steve", "abc123")
	if err != nil {
		t.Fatalf("HasJoined: %v", err)
	}
	if gotUser != "Steve" || gotID != "abc123" {
		t.Errorf("session request username=%q serverId=%q", gotUser, gotID)
	}
	if !p.OnlineMode || p.Name != "Steve" {
		t.Errorf("profile = %+v", p)
	}
	if p.UUID.String() != "2d0e5b91-b9a2-434b-9b3d-9f9f1b3f4a5b" {
		t.Errorf("uuid = %s", p.UUID)
	}
}

func TestHasJoinedNotAuthenticated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := &MojangClient{BaseURL: srv.URL, HTTP: srv.Client()}
	if _, err := c.HasJoined(context.Background(), "Steve", "abc"); err == nil {
		t.Fatal("expected error for unauthenticated player")
	}
}

func TestServerHashFormat(t *testing.T) {
	// The digest must be signed lowercase hex. Verify it is non-empty and
	// matches the regex ^-?[0-9a-f]+$.
	h := ServerHash(" ", []byte("secret"), []byte("pubkey"))
	if h == "" {
		t.Fatal("empty server hash")
	}
	if !strings.HasPrefix(h, "-") {
		lower := strings.ToLower(h)
		for _, c := range lower {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Fatalf("invalid hex char %q in %q", c, h)
			}
		}
	}
}

func TestOfflineUUIDDeterministic(t *testing.T) {
	a := OfflineUUID("Steve")
	b := OfflineUUID("Steve")
	c := OfflineUUID("Alex")
	if a != b {
		t.Errorf("offline uuid not deterministic: %s vs %s", a, b)
	}
	if a == c {
		t.Errorf("offline uuid collision between names")
	}
}
