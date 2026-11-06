package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ahmettsezis/flui/internal/auth"
	"github.com/ahmettsezis/flui/internal/collector"
	"github.com/ahmettsezis/flui/internal/config"
	"github.com/ahmettsezis/flui/internal/nodes"
	"github.com/ahmettsezis/flui/internal/store"
)

type env struct {
	t       *testing.T
	h       http.Handler
	db      *sql.DB
	adminPW string
}

func itoa(i int) string { return strconv.Itoa(i) }

func newEnv(t *testing.T) *env {
	t.Helper()
	ctx := context.Background()
	st, err := store.Open(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default()
	svc, err := auth.NewService(st.DB, cfg.Auth)
	if err != nil {
		t.Fatal(err)
	}
	pw, _, err := svc.Bootstrap(ctx)
	if err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.DiscardHandler)
	repo := nodes.NewRepo(st.DB)
	h := New(Options{
		DB: st, SQL: st.DB, Logger: log, UI: fstest.MapFS{}, Auth: svc, Nodes: repo,
		Collector: collector.New(st.DB, repo, cfg.Collector, log), Config: cfg,
	})
	return &env{t: t, h: h, db: st.DB, adminPW: pw}
}

type client struct {
	e      *env
	cookie string
	csrf   string
}

type resp struct {
	code int
	body map[string]any
	raw  string
	hdr  http.Header
}

func (c *client) do(method, path, body string, hdr ...string) resp {
	c.e.t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, "https://flui.test"+path, r)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.cookie != "" {
		req.AddCookie(&http.Cookie{Name: sessionCookie, Value: c.cookie})
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	rec := httptest.NewRecorder()
	c.e.h.ServeHTTP(rec, req)
	out := resp{code: rec.Code, raw: rec.Body.String(), hdr: rec.Header()}
	_ = json.Unmarshal(rec.Body.Bytes(), &out.body)
	return out
}

func (c *client) login(user, pw string) resp {
	c.e.t.Helper()
	r := c.do("POST", "/api/v1/auth/login", `{"username":"`+user+`","password":"`+pw+`"}`)
	if r.code == http.StatusOK {
		for _, ck := range (&http.Response{Header: r.hdr}).Cookies() {
			if ck.Name == sessionCookie {
				c.cookie = ck.Value
				if !ck.HttpOnly || !ck.Secure || ck.SameSite != http.SameSiteStrictMode || ck.Path != "/" {
					c.e.t.Errorf("insecure session cookie: %+v", ck)
				}
			}
		}
		c.csrf, _ = r.body["csrf_token"].(string)
	}
	return r
}

func expect(t *testing.T, name string, r resp, code int) {
	t.Helper()
	if r.code != code {
		t.Fatalf("%s: status %d, want %d (%s)", name, r.code, code, r.raw)
	}
}

func TestAuthAndNodeFlow(t *testing.T) {
	e := newEnv(t)
	anon := &client{e: e}
	expect(t, "nodes without session", anon.do("GET", "/api/v1/nodes", ""), 401)
	expect(t, "form login", anon.do("POST", "/api/v1/auth/login", "username=admin", "Content-Type", "application/x-www-form-urlencoded"), 403)
	expect(t, "cross-site login", anon.do("POST", "/api/v1/auth/login", `{"username":"admin","password":"x"}`, "Origin", "https://evil.example"), 403)
	expect(t, "bad password", anon.login("admin", "wrong"), 401)
	expect(t, "unknown field", anon.do("POST", "/api/v1/auth/login", `{"username":"a","password":"b","x":1}`), 400)

	admin := &client{e: e}
	r := admin.login("admin", e.adminPW)
	expect(t, "login", r, 200)
	if u := r.body["user"].(map[string]any); u["must_change_password"] != true || u["role"] != "admin" {
		t.Fatalf("user = %v", u)
	}
	expect(t, "nodes before password change", admin.do("GET", "/api/v1/nodes", ""), 403)
	expect(t, "me before password change", admin.do("GET", "/api/v1/auth/me", ""), 200)

	newPW := `{"current_password":"` + e.adminPW + `","new_password":"a-long-new-password"}`
	noCSRF := &client{e: e, cookie: admin.cookie}
	expect(t, "password change without CSRF", noCSRF.do("POST", "/api/v1/auth/password", newPW), 403)
	expect(t, "weak password", admin.do("POST", "/api/v1/auth/password", `{"current_password":"`+e.adminPW+`","new_password":"short"}`), 400)
	expect(t, "password change", admin.do("POST", "/api/v1/auth/password", newPW), 204)

	expect(t, "invalid node", admin.do("POST", "/api/v1/nodes", `{"name":"web 1","monitor_url":"ftp://x"}`), 400)
	r = admin.do("POST", "/api/v1/nodes", `{"name":"web-1","monitor_url":"127.0.0.1:1","workers":1}`)
	expect(t, "create node", r, 201)
	id := int(r.body["id"].(float64))
	if !strings.Contains(r.body["snippet"].(string), "monitor_agent") {
		t.Error("snippet missing")
	}
	expect(t, "duplicate node", admin.do("POST", "/api/v1/nodes", `{"name":"WEB-1","monitor_url":"127.0.0.1:2"}`), 409)
	expect(t, "cross-origin create", admin.do("POST", "/api/v1/nodes", `{"name":"x","monitor_url":"127.0.0.1"}`, "Origin", "https://evil.example"), 403)
	expect(t, "cross-site fetch", admin.do("POST", "/api/v1/nodes", `{"name":"x","monitor_url":"127.0.0.1"}`, "Sec-Fetch-Site", "cross-site"), 403)

	r = admin.do("POST", "/api/v1/nodes/test", `{"monitor_url":"127.0.0.1:1"}`)
	expect(t, "test unreachable", r, 200)
	if r.body["ok"] != false || r.body["error"] == "" {
		t.Errorf("test result = %v", r.body)
	}

	r = admin.do("GET", "/api/v1/nodes", "")
	expect(t, "list nodes", r, 200)
	if !strings.Contains(r.raw, `"name":"web-1"`) || !strings.Contains(r.raw, `"status":{"state"`) {
		t.Errorf("list = %s", r.raw)
	}
	path := "/api/v1/nodes/" + itoa(id)
	expect(t, "get node", admin.do("GET", path, ""), 200)
	expect(t, "metrics", admin.do("GET", path+"/metrics?range=6h", ""), 200)
	expect(t, "metrics bad range", admin.do("GET", path+"/metrics?range=1y", ""), 400)
	expect(t, "update node", admin.do("PUT", path, `{"name":"web-01","monitor_url":"127.0.0.1:3","workers":2}`), 200)
	expect(t, "missing node", admin.do("GET", "/api/v1/nodes/999", ""), 404)

	hash, _ := auth.HashPassword("viewer-password-123")
	now := time.Now().Unix()
	if _, err := e.db.Exec(`INSERT INTO users (username, password_hash, role, created_at, updated_at) VALUES ('view', ?, 'viewer', ?, ?)`, hash, now, now); err != nil {
		t.Fatal(err)
	}
	viewer := &client{e: e}
	expect(t, "viewer login", viewer.login("view", "viewer-password-123"), 200)
	expect(t, "viewer list", viewer.do("GET", "/api/v1/nodes", ""), 200)
	expect(t, "viewer create", viewer.do("POST", "/api/v1/nodes", `{"name":"x","monitor_url":"127.0.0.1"}`), 403)
	expect(t, "viewer delete", viewer.do("DELETE", path, ""), 403)

	expect(t, "delete node", admin.do("DELETE", path, ""), 204)
	expect(t, "logout", admin.do("POST", "/api/v1/auth/logout", ""), 204)
	expect(t, "after logout", admin.do("GET", "/api/v1/nodes", ""), 401)

	var actions []string
	rows, _ := e.db.Query(`SELECT action FROM audit_log ORDER BY id`)
	for rows.Next() {
		var a string
		_ = rows.Scan(&a)
		actions = append(actions, a)
	}
	got := strings.Join(actions, ",")
	for _, want := range []string{"user.bootstrap", "auth.login_failed", "auth.login", "auth.password_change", "node.create", "node.update", "node.delete", "auth.logout"} {
		if !strings.Contains(got, want) {
			t.Errorf("audit log missing %s: %s", want, got)
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	e := newEnv(t)
	c := &client{e: e}
	for i := range 10 {
		expect(t, "attempt "+itoa(i), c.login("nobody", "x"), 401)
	}
	r := c.login("nobody", "x")
	expect(t, "rate limited", r, 429)
	if r.hdr.Get("Retry-After") == "" {
		t.Error("Retry-After missing")
	}
}

func TestClientIP(t *testing.T) {
	srv := &server{trusted: []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}}
	cases := []struct{ remote, xff, want string }{
		{"203.0.113.5:1234", "1.2.3.4", "203.0.113.5"},
		{"10.0.0.2:1234", "1.2.3.4", "1.2.3.4"},
		{"10.0.0.2:1234", "1.2.3.4, 10.0.0.9", "1.2.3.4"},
		{"10.0.0.2:1234", "garbage", "10.0.0.2"},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = c.remote
		req.Header.Set("X-Forwarded-For", c.xff)
		if got := srv.clientIP(req); got != c.want {
			t.Errorf("remote=%s xff=%q: got %s, want %s", c.remote, c.xff, got, c.want)
		}
	}
}
