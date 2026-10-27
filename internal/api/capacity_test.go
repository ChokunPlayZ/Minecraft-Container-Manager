package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/docker"
	"github.com/mcm-panel/mcm/internal/servers"
)

func TestHostCapacityEndpoint(t *testing.T) {
	store, err := db.Open(filepath.Join(t.TempDir(), "mcm.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	users := auth.NewUsers(store.DB)
	user, err := users.Create(context.Background(), "capacity@example.test", "hash")
	if err != nil {
		t.Fatal(err)
	}
	token := mustSession(t, store, user.ID)
	for _, runtime := range []string{"docker", "podman"} {
		t.Run(runtime, func(t *testing.T) {
			var fail atomic.Bool
			daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/_ping" {
					w.Header().Set("API-Version", "1.40")
					return
				}
				if r.URL.Path != "/v1.40/info" {
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if fail.Load() {
					w.WriteHeader(http.StatusServiceUnavailable)
					_, _ = w.Write([]byte(`{"message":"unreachable"}`))
					return
				}
				_, _ = w.Write([]byte(`{"NCPU":16,"MemTotal":68719476736}`))
			}))
			t.Cleanup(daemon.Close)
			manager, err := docker.NewWithRuntime(runtime, "tcp://"+strings.TrimPrefix(daemon.URL, "http://"), "")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = manager.Close() })
			handler := New(Options{DB: store, Users: users, Sessions: auth.NewManager(store.DB), Servers: servers.NewStore(store, manager, nil, 25565, 25665, t.TempDir(), "")})
			req := httptest.NewRequest(http.MethodGet, "/api/system/capacity", nil)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("unauthenticated = %d", response.Code)
			}
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			response = httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != http.StatusOK {
				t.Fatalf("capacity = %d: %s", response.Code, response.Body)
			}
			var capacity servers.HostResources
			if err := json.Unmarshal(response.Body.Bytes(), &capacity); err != nil {
				t.Fatal(err)
			}
			if capacity.Runtime != runtime || capacity.CPUCores != 16 || capacity.MemoryTotalBytes != 64<<30 {
				t.Fatalf("capacity = %+v", capacity)
			}
			if capacity.Disk == nil || capacity.Disk.TotalBytes == 0 || capacity.Usage == nil || capacity.Usage.MemoryBytes != 0 {
				t.Fatalf("missing disk or idle-server usage: %+v", capacity)
			}
			fail.Store(true)
			response = httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			if response.Code != http.StatusOK {
				t.Fatalf("unavailable capacity = %d: %s", response.Code, response.Body)
			}
			if err := json.Unmarshal(response.Body.Bytes(), &capacity); err != nil {
				t.Fatal(err)
			}
			if capacity.RuntimeError == "" || capacity.Usage != nil || capacity.Disk == nil {
				t.Fatalf("runtime failure should preserve disk space: %+v", capacity)
			}
		})
	}
	for _, serverStore := range []*servers.Store{nil, servers.NewStore(store, nil, nil, 25565, 25665, t.TempDir(), "")} {
		handler := New(Options{DB: store, Users: users, Sessions: auth.NewManager(store.DB), Servers: serverStore})
		req := httptest.NewRequest(http.MethodGet, "/api/system/capacity", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		if serverStore == nil && response.Code != http.StatusServiceUnavailable {
			t.Fatalf("missing runtime = %d: %s", response.Code, response.Body)
		}
		if serverStore != nil && (response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "runtime_error")) {
			t.Fatalf("disk-only response = %d: %s", response.Code, response.Body)
		}
	}
}
