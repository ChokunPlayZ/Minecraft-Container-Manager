package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/backups"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/jars"
	"github.com/mcm-panel/mcm/internal/servers"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReviewEncodedServerIDCannotEscapeDataRoot(t *testing.T) {
	dir := t.TempDir()
	d, e := db.Open(filepath.Join(dir, "mcm.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("panel secret"), 0644)
	users := auth.NewUsers(d.DB)
	u, e := users.Create(context.Background(), "review@example.test", "test-hash")
	if e != nil {
		t.Fatal(e)
	}
	sessions := auth.NewManager(d.DB)
	token, e := sessions.Create(context.Background(), u.ID)
	if e != nil {
		t.Fatal(e)
	}
	h := New(Options{DB: d, Servers: servers.NewStore(d, nil, nil, 25565, 25575, dir, ""), Users: users, Sessions: sessions})
	r := httptest.NewRequest(http.MethodGet, "/api/servers/%2e%2e/files/content?path=secret.txt", nil)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code == 200 && strings.Contains(w.Body.String(), "panel secret") {
		t.Errorf("authenticated encoded-ID traversal returned panel data outside server root: %s", w.Body.String())
	}
}
func TestReviewConcurrentOnboardingCreatesOnlyOneAdmin(t *testing.T) {
	dir := t.TempDir()
	d, e := db.Open(filepath.Join(dir, "mcm.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	users := auth.NewUsers(d.DB)
	sessions := auth.NewManager(d.DB)
	h := New(Options{DB: d, Users: users, Sessions: sessions})
	start := make(chan struct{})
	codes := make(chan int, 2)
	for _, email := range []string{"alice@example.test", "bob@example.test"} {
		go func(email string) {
			<-start
			r := httptest.NewRequest(http.MethodPost, "/api/onboarding", strings.NewReader(`{"email":"`+email+`","password":"password123"}`))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			codes <- w.Code
		}(email)
	}
	close(start)
	a, b := <-codes, <-codes
	n, e := users.Count(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	if n != 1 {
		t.Errorf("concurrent onboarding created %d administrators; HTTP statuses %d,%d", n, a, b)
	}
}
func TestReviewFailedModReplacementKeepsExistingJar(t *testing.T) {
	dir := t.TempDir()
	d, e := db.Open(filepath.Join(dir, "mcm.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	_, e = d.DB.Exec(`INSERT INTO servers(id,name,server_type,version,ram_mb,host_port,state,created_at,updated_at) VALUES('s','Server','fabric','1.21.1',2048,25565,'stopped','now','now')`)
	if e != nil {
		t.Fatal(e)
	}
	moddir := filepath.Join(dir, "servers/s/mods")
	os.MkdirAll(moddir, 0755)
	old := filepath.Join(moddir, "old.jar")
	os.WriteFile(old, []byte("old"), 0644)
	s := &Server{servers: servers.NewStore(d, nil, nil, 25565, 25575, dir, "")}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"url":"file://invalid","filename":"new.jar","delete_old_name":"old.jar"}`))
	r.SetPathValue("id", "s")
	w := httptest.NewRecorder()
	s.handleDownloadMod(w, r)
	if _, e = os.Stat(old); e != nil {
		t.Errorf("failed replacement HTTP %d removed old mod: %v", w.Code, e)
	}
}

func TestInstallPOSTCompletesAndLargeEditorSaves(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	users := auth.NewUsers(database.DB)
	user, err := users.Create(ctx, "admin@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewManager(database.DB)
	token, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	store := servers.NewStore(database, nil, jars.NewResolver(), 25565, 25575, dir, "")
	if _, err = database.DB.Exec(`INSERT INTO servers(id,name,server_type,version,ram_mb,host_port,state,created_at,updated_at) VALUES('server','Test','custom','custom',1024,0,'stopped','now','now')`); err != nil {
		t.Fatal(err)
	}
	handler := New(Options{DB: database, Servers: store, Users: users, Sessions: sessions})
	call := func(method, url string, body io.Reader) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, url, body)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "csrf-token"})
		req.Header.Set(csrfHeaderName, "csrf-token")
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, req)
		return result
	}
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() { done <- call(http.MethodPost, "/api/servers/server/install", strings.NewReader(`{}`)) }()
	select {
	case result := <-done:
		if result.Code != http.StatusCreated {
			t.Fatalf("install: %d %s", result.Code, result.Body.String())
		}
	case <-time.After(time.Second):
		t.Fatal("POST install deadlocked")
	}
	content := strings.Repeat("\x01", 2<<20)
	body, _ := json.Marshal(map[string]string{"path": "config.txt", "content": content})
	result := call(http.MethodPut, "/api/servers/server/files/content", bytes.NewReader(body))
	if result.Code != http.StatusCreated {
		t.Fatalf("large editor save: %d %s", result.Code, result.Body.String())
	}
	data, err := store.ReadFileContent("server", "config.txt")
	if err != nil || data != content {
		t.Fatal("editor save lost bytes", err)
	}
}

func TestRejectedUserUpdateLeavesIdentityUnchanged(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	users := auth.NewUsers(database.DB)
	user, err := users.Create(context.Background(), "before@example.test", "old-hash")
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{users: users}
	req := httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"email":"after@example.test","password":"short"}`))
	req.SetPathValue("id", user.ID)
	result := httptest.NewRecorder()
	server.handleUpdateUser(result, req)
	if result.Code != http.StatusBadRequest {
		t.Fatal(result.Code)
	}
	got, err := users.GetByID(context.Background(), user.ID)
	if err != nil || got.Email != user.Email || got.PasswordHash != user.PasswordHash {
		t.Fatalf("rejected update mutated account: %+v %v", got, err)
	}
}

func TestRestoreRejectsBackupFromAnotherServer(t *testing.T) {
	dir := t.TempDir()
	database, err := db.Open(filepath.Join(dir, "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, id := range []string{"a", "b"} {
		if _, err = database.DB.Exec(`INSERT INTO servers(id,name,server_type,version,ram_mb,host_port,state,created_at,updated_at) VALUES(?,?,'custom','custom',1024,0,'stopped','now','now')`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = database.DB.Exec(`INSERT INTO backups(id,server_id,name,size_bytes,location,status,created_at) VALUES('backup-b','b','B',0,'local:missing','completed','now')`); err != nil {
		t.Fatal(err)
	}
	server := &Server{servers: servers.NewStore(database, nil, nil, 25565, 25575, dir, ""), backups: backups.New(database.DB, backups.S3Config{}, dir)}
	req := httptest.NewRequest(http.MethodPost, "/", nil)
	req.SetPathValue("id", "a")
	req.SetPathValue("backupId", "backup-b")
	result := httptest.NewRecorder()
	server.handleRestoreBackup(result, req)
	if result.Code != http.StatusBadRequest || !strings.Contains(result.Body.String(), "backup_server_mismatch") {
		t.Fatalf("restore scope: %d %s", result.Code, result.Body.String())
	}
}
