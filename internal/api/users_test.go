package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
)

type userJSON struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	TOTPEnabled bool   `json:"totp_enabled"`
	CreatedAt   string `json:"created_at"`
}

func newUsersTestServer(t *testing.T) (*db.Store, http.Handler) {
	t.Helper()
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	users := auth.NewUsers(store.DB)
	sessions := auth.NewManager(store.DB)
	passkeys := auth.NewPasskeys(store.DB)

	handler := New(Options{
		DB:       store,
		Users:    users,
		Sessions: sessions,
		Passkeys: passkeys,
	})
	return store, handler
}

func TestUsersCreateListUpdateDelete(t *testing.T) {
	store, handler := newUsersTestServer(t)
	ctx := context.Background()
	users := auth.NewUsers(store.DB)

	// Seed an acting admin and an extra user via the repo (not the API).
	admin, err := users.Create(ctx, "admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := users.Create(ctx, "extra@example.test", "hash"); err != nil {
		t.Fatalf("create extra: %v", err)
	}

	// List users (GET is CSRF-exempt).
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mustSession(t, store, admin.ID)})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var list struct {
		Users []userJSON `json:"users"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v; body=%s", err, rr.Body.String())
	}
	if len(list.Users) != 2 {
		t.Fatalf("expected 2 users, got %d: %+v", len(list.Users), list.Users)
	}

	// Create a user (POST needs CSRF).
	csrf := mustCSRF(t)
	req = httptest.NewRequest(http.MethodPost, "/api/users",
		bytes.NewBufferString(`{"email":"new@example.test","password":"password123"}`))
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mustSession(t, store, admin.ID)})
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var created userJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode created: %v", err)
	}
	if created.Email != "new@example.test" {
		t.Fatalf("created email = %q", created.Email)
	}

	// Update email (PATCH).
	csrf = mustCSRF(t)
	req = httptest.NewRequest(http.MethodPatch, "/api/users/"+created.ID,
		bytes.NewBufferString(`{"email":"updated@example.test"}`))
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mustSession(t, store, admin.ID)})
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("update status = %d, body=%s", rr.Code, rr.Body.String())
	}
	var updated userJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &updated); err != nil {
		t.Fatalf("decode updated: %v", err)
	}
	if updated.Email != "updated@example.test" {
		t.Fatalf("updated email = %q", updated.Email)
	}

	// Delete user (DELETE).
	csrf = mustCSRF(t)
	req = httptest.NewRequest(http.MethodDelete, "/api/users/"+created.ID, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mustSession(t, store, admin.ID)})
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("delete status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if _, err := users.GetByID(ctx, created.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("expected user deleted, got %v", err)
	}
}

func TestUsersCannotDeleteLastUser(t *testing.T) {
	store, handler := newUsersTestServer(t)
	ctx := context.Background()
	users := auth.NewUsers(store.DB)
	admin, err := users.Create(ctx, "solo@example.test", "hash")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	// With a single account, the last-user guard rejects deletion of a
	// non-self target before it can reach the delete path.
	csrf := mustCSRF(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/users/some-other-id", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mustSession(t, store, admin.ID)})
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if !isFriendlyErrorBody(t, rr.Body.Bytes()) {
		t.Fatalf("expected friendly error, got %s", rr.Body.String())
	}
	if _, err := users.GetByID(ctx, admin.ID); err != nil {
		t.Fatalf("admin should not be affected, got %v", err)
	}
}

func TestUsersCannotDeleteSelf(t *testing.T) {
	store, handler := newUsersTestServer(t)
	ctx := context.Background()
	users := auth.NewUsers(store.DB)
	admin, err := users.Create(ctx, "self@example.test", "hash")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := users.Create(ctx, "other@example.test", "hash"); err != nil {
		t.Fatalf("create other: %v", err)
	}

	csrf := mustCSRF(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/users/"+admin.ID, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mustSession(t, store, admin.ID)})
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if _, err := users.GetByID(ctx, admin.ID); err != nil {
		t.Fatalf("self should not be deleted, got %v", err)
	}
}

func TestUsersCreateRequiresMinPassword(t *testing.T) {
	store, handler := newUsersTestServer(t)
	ctx := context.Background()
	users := auth.NewUsers(store.DB)
	admin, err := users.Create(ctx, "admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}

	csrf := mustCSRF(t)
	req := httptest.NewRequest(http.MethodPost, "/api/users",
		bytes.NewBufferString(`{"email":"short@example.test","password":"short"}`))
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mustSession(t, store, admin.ID)})
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body=%s", rr.Code, rr.Body.String())
	}
	if !isFriendlyErrorBody(t, rr.Body.Bytes()) {
		t.Fatalf("expected friendly error, got %s", rr.Body.String())
	}
}

func TestUsersUpdatePasswordPreservesOwnSession(t *testing.T) {
	store, handler := newUsersTestServer(t)
	ctx := context.Background()
	users := auth.NewUsers(store.DB)
	sessions := auth.NewManager(store.DB)
	admin, err := users.Create(ctx, "admin@example.test", "hash")
	if err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if _, err := users.Create(ctx, "other@example.test", "hash"); err != nil {
		t.Fatalf("create other: %v", err)
	}

	// The acting session must survive a self password change.
	selfToken := mustSession(t, store, admin.ID)
	csrf := mustCSRF(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/users/"+admin.ID,
		bytes.NewBufferString(`{"password":"newpassword123"}`))
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: selfToken})
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if _, err := sessions.Validate(ctx, selfToken); err != nil {
		t.Fatalf("own session should be preserved, got %v", err)
	}

	// An admin changing another user's password revokes that user's sessions.
	other, err := users.GetByEmail(ctx, "other@example.test")
	if err != nil {
		t.Fatalf("get other: %v", err)
	}
	otherToken := mustSession(t, store, other.ID)
	csrf = mustCSRF(t)
	req = httptest.NewRequest(http.MethodPatch, "/api/users/"+other.ID,
		bytes.NewBufferString(`{"password":"anotherpass123"}`))
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: mustSession(t, store, admin.ID)})
	req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: csrf})
	req.Header.Set(csrfHeaderName, csrf)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if _, err := sessions.Validate(ctx, otherToken); !errors.Is(err, auth.ErrSessionNotFound) {
		t.Fatalf("expected other sessions revoked, got %v", err)
	}
}

func mustSession(t *testing.T, store *db.Store, userID string) string {
	t.Helper()
	sessions := auth.NewManager(store.DB)
	token, err := sessions.Create(context.Background(), userID)
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	return token
}

func mustCSRF(t *testing.T) string {
	t.Helper()
	tok, err := generateCSRFToken()
	if err != nil {
		t.Fatalf("generate csrf: %v", err)
	}
	return tok
}
