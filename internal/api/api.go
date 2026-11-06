// Package api implements flui's HTTP API and serves the web UI.
package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"github.com/ahmettsezis/flui/internal/auth"
	"github.com/ahmettsezis/flui/internal/collector"
	"github.com/ahmettsezis/flui/internal/config"
	"github.com/ahmettsezis/flui/internal/nodes"
)

type Pinger interface {
	Ping(ctx context.Context) error
}

type Options struct {
	DB        Pinger
	SQL       *sql.DB
	Logger    *slog.Logger
	UI        fs.FS
	Auth      *auth.Service
	Nodes     *nodes.Repo
	Collector *collector.Collector
	Config    config.Config
	// OnPasswordChanged is called after a user changed their password (e.g. to remove the initial password file).
	OnPasswordChanged func(*auth.User)
}

type server struct {
	Options
	trusted      []netip.Prefix
	loginLimiter *auth.RateLimiter
}

func New(o Options) http.Handler {
	s := &server{Options: o, loginLimiter: auth.NewRateLimiter(10, time.Minute)}
	for _, p := range o.Config.Server.TrustedProxies {
		if pfx, err := netip.ParsePrefix(p); err == nil {
			s.trusted = append(s.trusted, pfx)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", s.readyz)

	if o.Auth != nil {
		viewer, operator := s.require(auth.RoleViewer, false), s.require(auth.RoleOperator, false)
		anyUser := s.require(auth.RoleViewer, true)

		mux.HandleFunc("POST /api/v1/auth/login", s.login)
		mux.Handle("GET /api/v1/auth/me", anyUser(s.me))
		mux.Handle("POST /api/v1/auth/logout", anyUser(s.logout))
		mux.Handle("POST /api/v1/auth/password", anyUser(s.changePassword))

		mux.Handle("GET /api/v1/nodes", viewer(s.listNodes))
		mux.Handle("POST /api/v1/nodes", operator(s.createNode))
		mux.Handle("POST /api/v1/nodes/test", operator(s.testNode))
		mux.Handle("GET /api/v1/nodes/{id}", viewer(s.getNode))
		mux.Handle("PUT /api/v1/nodes/{id}", operator(s.updateNode))
		mux.Handle("DELETE /api/v1/nodes/{id}", operator(s.deleteNode))
		mux.Handle("GET /api/v1/nodes/{id}/metrics", viewer(s.nodeMetrics))
	}
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeError(w, http.StatusNotFound, "not found")
	})
	mux.Handle("/", uiHandler(o.UI))

	return logRequests(o.Logger, recoverPanics(o.Logger, securityHeaders(mux)))
}

func (s *server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := s.DB.Ping(ctx); err != nil {
		s.Logger.Warn("readiness check failed", "err", err)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

const maxJSONBody = 64 << 10

// decodeJSON parses a JSON request body strictly. It writes an error response and returns false on failure.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			writeError(w, http.StatusBadRequest, "invalid JSON body")
		}
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *server) internalError(w http.ResponseWriter, r *http.Request, err error) {
	s.Logger.Error("request failed", "path", r.URL.Path, "err", err)
	writeError(w, http.StatusInternalServerError, "internal error")
}
