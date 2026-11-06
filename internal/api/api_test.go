package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

type fakeDB struct{ err error }

func (f fakeDB) Ping(context.Context) error { return f.err }

func newHandler(db Pinger, ui fstest.MapFS) http.Handler {
	return New(Options{DB: db, Logger: slog.New(slog.DiscardHandler), UI: ui})
}

var testUI = fstest.MapFS{
	"index.html":       {Data: []byte("<html>app</html>")},
	"assets/app-1.js":  {Data: []byte("console.log(1)")},
	"favicon.svg":      {Data: []byte("<svg/>")},
	"assets/style.css": {Data: []byte("body{}")},
}

func get(t *testing.T, h http.Handler, path string) *http.Response {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	return rec.Result()
}

func body(t *testing.T, r *http.Response) string {
	t.Helper()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestHealthAndReadiness(t *testing.T) {
	h := newHandler(fakeDB{}, testUI)
	if r := get(t, h, "/healthz"); r.StatusCode != http.StatusOK || !strings.Contains(body(t, r), `"ok"`) {
		t.Errorf("healthz = %d", r.StatusCode)
	}
	if r := get(t, h, "/readyz"); r.StatusCode != http.StatusOK {
		t.Errorf("readyz = %d", r.StatusCode)
	}
	down := newHandler(fakeDB{err: errors.New("db gone")}, testUI)
	if r := get(t, down, "/readyz"); r.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("readyz with failing db = %d", r.StatusCode)
	}
}

func TestSecurityHeaders(t *testing.T) {
	h := newHandler(fakeDB{}, testUI)
	for _, p := range []string{"/", "/healthz", "/api/nope", "/assets/app-1.js"} {
		r := get(t, h, p)
		for _, k := range []string{"Strict-Transport-Security", "Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
			if r.Header.Get(k) == "" {
				t.Errorf("%s: missing %s", p, k)
			}
		}
		if csp := r.Header.Get("Content-Security-Policy"); strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
			t.Errorf("%s: weak CSP %q", p, csp)
		}
	}
}

func TestUIRouting(t *testing.T) {
	h := newHandler(fakeDB{}, testUI)
	cases := []struct {
		path, wantBody, wantCache string
		wantStatus                int
	}{
		{"/", "<html>app</html>", "no-cache", 200},
		{"/nodes/42", "<html>app</html>", "no-cache", 200},
		{"/assets/app-1.js", "console.log(1)", "immutable", 200},
		{"/favicon.svg", "<svg/>", "no-cache", 200},
		{"/missing.png", "", "", 404},
		{"/../../etc/passwd", "", "", http.StatusTemporaryRedirect},
	}
	for _, c := range cases {
		r := get(t, h, c.path)
		b := body(t, r)
		if r.StatusCode != c.wantStatus || (c.wantBody != "" && b != c.wantBody) || !strings.Contains(r.Header.Get("Cache-Control"), c.wantCache) {
			t.Errorf("%s: status=%d body=%q cache=%q", c.path, r.StatusCode, b, r.Header.Get("Cache-Control"))
		}
	}
	if r := get(t, h, "/api/unknown"); r.StatusCode != 404 || r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("/api/unknown: status=%d type=%q", r.StatusCode, r.Header.Get("Content-Type"))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /: status=%d", rec.Code)
	}
}

func TestUINotBuilt(t *testing.T) {
	h := newHandler(fakeDB{}, fstest.MapFS{})
	if r := get(t, h, "/"); r.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d", r.StatusCode)
	}
}

func TestPanicRecovery(t *testing.T) {
	h := recoverPanics(slog.New(slog.DiscardHandler), http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("boom") }))
	if r := get(t, h, "/"); r.StatusCode != http.StatusInternalServerError || strings.Contains(body(t, r), "boom") {
		t.Errorf("status = %d", r.StatusCode)
	}
}
