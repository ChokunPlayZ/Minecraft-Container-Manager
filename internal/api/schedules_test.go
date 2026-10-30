package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcm-panel/mcm/internal/auth"
	"github.com/mcm-panel/mcm/internal/db"
	"github.com/mcm-panel/mcm/internal/schedules"
)

func TestScheduleEndpoints(t *testing.T) {
	t.Setenv("TZ", "Asia/Bangkok")
	database, err := db.Open(filepath.Join(t.TempDir(), "api.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	ctx := context.Background()
	users := auth.NewUsers(database.DB)
	user, err := users.Create(ctx, "admin@example.com", "password123")
	if err != nil {
		t.Fatal(err)
	}
	sessions := auth.NewManager(database.DB)
	token, err := sessions.Create(ctx, user.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		if _, err := database.DB.Exec(`INSERT INTO servers (id,name,server_type,version,ram_mb,created_at,updated_at) VALUES (?,?,'paper','1.21.4',2048,'now','now')`, id, id); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(Options{DB: database, Users: users, Sessions: sessions})
	request := func(method, target, body string, authenticated, csrf bool, want int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, target, strings.NewReader(body))
		if authenticated {
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		}
		if csrf {
			r.AddCookie(&http.Cookie{Name: CSRFCookieName, Value: "test-csrf"})
			r.Header.Set(csrfHeaderName, "test-csrf")
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: %d %s; want %d", method, target, w.Code, w.Body.String(), want)
		}
		return w
	}
	base := "/api/servers/one/schedules"
	payload := `{"name":"Midnight restart","action":"restart","cron":"midnight"}`
	request("GET", base, "", false, false, 401)
	request("POST", base, payload, false, true, 401)
	request("POST", base, payload, true, false, 403)
	request("POST", base, `{`, true, true, 400)
	request("POST", base, `{"name":"bad","action":"command","command":"say hi","cron":"99 * * * *","timezone":"UTC"}`, true, true, 400)
	created := request("POST", base, payload, true, true, 201)
	var task schedules.Task
	if err := json.Unmarshal(created.Body.Bytes(), &task); err != nil {
		t.Fatal(err)
	}
	if !task.Enabled || task.NextRunAt == "" || task.Timezone != "Asia/Bangkok" {
		t.Fatalf("default enabled failed: %+v", task)
	}
	path := base + "/" + task.ID
	request("PUT", path, payload, false, true, 401)
	request("PATCH", path, `{"enabled":false}`, false, true, 401)
	request("DELETE", path, "", false, true, 401)
	request("PATCH", path, `{}`, true, true, 400)
	request("PATCH", path, `{"enabled":false}`, true, true, 200)
	request("PATCH", path, `{"enabled":true}`, true, true, 200)
	request("PUT", path, `{"name":"Save world","action":"command","command":"save-all","cron":"*/30 * * * *","timezone":"UTC","enabled":true}`, true, true, 200)
	request("GET", "/api/servers/missing/schedules", "", true, false, 404)
	request("PUT", "/api/servers/two/schedules/"+task.ID, payload, true, true, 404)
	request("DELETE", "/api/servers/two/schedules/"+task.ID, "", true, true, 404)
	list := request("GET", base, "", true, false, 200)
	var body struct {
		Schedules       []schedules.Task `json:"schedules"`
		DefaultTimezone string           `json:"default_timezone"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &body); err != nil || len(body.Schedules) != 1 || body.Schedules[0].Command != "save-all" || body.DefaultTimezone != "Asia/Bangkok" {
		t.Fatalf("list: %s %v", list.Body.String(), err)
	}
	request("DELETE", path, "", true, true, 200)
	request("DELETE", path, "", true, true, 404)
	empty := request("GET", base, "", true, false, 200)
	if !strings.Contains(empty.Body.String(), `"schedules":[]`) {
		t.Fatal(empty.Body.String())
	}
	backup := request("POST", base, `{"name":"Automatic backup","action":"backup","backup_target":"s3","cron":"@every 95m","enabled":false}`, true, true, 201)
	if err := json.Unmarshal(backup.Body.Bytes(), &task); err != nil || task.Action != "backup" || task.Cron != "@every 95m" || task.Enabled || task.NextRunAt != "" || task.BackupTarget != "s3" {
		t.Fatalf("backup schedule: %s %v", backup.Body.String(), err)
	}
	request("PATCH", base+"/"+task.ID, `{"enabled":true}`, true, true, 200)
	request("POST", base, `{"name":"Too frequent","action":"backup","cron":"@every 1s"}`, true, true, 400)
	request("POST", base, `{"name":"Bad target","action":"backup","backup_target":"unknown","cron":"midnight"}`, true, true, 400)
	chain := request("POST", base, `{"name":"Announced backup","action":"chain","cron":"midnight","steps":[{"action":"command","command":"say Backup Starting"},{"action":"command","command":"save-all"},{"action":"backup","backup_target":"local"},{"action":"delay","delay_seconds":30},{"action":"command","command":"say Backup Complete"}]}`, true, true, 201)
	if err := json.Unmarshal(chain.Body.Bytes(), &task); err != nil || task.Action != "chain" || len(task.Steps) != 5 || task.Steps[2].BackupTarget != "local" || task.Steps[3].DelaySeconds != 30 || !task.Enabled {
		t.Fatalf("chain schedule: %s %v", chain.Body.String(), err)
	}
	chainPath := base + "/" + task.ID
	request("PUT", "/api/servers/two/schedules/"+task.ID, chain.Body.String(), true, true, 404)
	edited := request("PUT", chainPath, `{"name":"Restart and announce","action":"chain","cron":"hourly","enabled":false,"steps":[{"action":"restart"},{"action":"delay","delay_seconds":60},{"action":"command","command":"say Welcome back"}]}`, true, true, 200)
	if err := json.Unmarshal(edited.Body.Bytes(), &task); err != nil || len(task.Steps) != 3 || task.Steps[0].Action != "restart" || task.Enabled || task.NextRunAt != "" {
		t.Fatalf("edited chain: %s %v", edited.Body.String(), err)
	}
	request("PATCH", chainPath, `{"enabled":true}`, true, true, 200)
	request("POST", base, `{"name":"Empty chain","action":"chain","cron":"midnight","steps":[]}`, true, true, 400)
	request("POST", base, `{"name":"Nested chain","action":"chain","cron":"midnight","steps":[{"action":"chain"}]}`, true, true, 400)
	request("POST", base, `{"name":"Invalid command","action":"chain","cron":"midnight","steps":[{"action":"command","command":"say hi\nstop"}]}`, true, true, 400)
	request("POST", base, `{"name":"Invalid wait","action":"chain","cron":"midnight","steps":[{"action":"delay","delay_seconds":1.5}]}`, true, true, 400)
	request("DELETE", chainPath, "", true, true, 200)
}
