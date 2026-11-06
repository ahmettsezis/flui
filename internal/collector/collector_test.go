package collector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahmettsezis/flui/internal/config"
	"github.com/ahmettsezis/flui/internal/nodes"
	"github.com/ahmettsezis/flui/internal/store"
)

func setup(t *testing.T) (*sql.DB, *nodes.Repo, *Collector) {
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
	repo := nodes.NewRepo(st.DB)
	return st.DB, repo, New(st.DB, repo, config.Default().Collector, slog.New(slog.DiscardHandler))
}

// fakeFluentd serves the fixture with emit_records/write_count increased by step on every request.
func fakeFluentd(t *testing.T, step int64) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	fixture, err := os.ReadFile("testdata/plugins.json")
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/plugins.json" {
			http.NotFound(w, r)
			return
		}
		n := calls.Add(1) - 1
		body := strings.Replace(string(fixture), `"emit_records": 100`, fmt.Sprintf(`"emit_records": %d`, 100+n*step), 1)
		body = strings.Replace(body, `"write_count": 5`, fmt.Sprintf(`"write_count": %d`, 5+n), 1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestFetchPluginsKeepsOutputsOnly(t *testing.T) {
	srv, _ := fakeFluentd(t, 0)
	ps, err := fetchPlugins(context.Background(), newHTTPClient(time.Second), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0].PluginID != "out_es" || *ps[0].BufferTotalQueuedSize != 1024 || *ps[0].BufferAvailableRatio != 99.9 {
		t.Fatalf("plugins = %+v", ps)
	}
	if ps[1].BufferQueueLength != nil {
		t.Error("absent value should be nil")
	}
}

func TestFetchErrors(t *testing.T) {
	redirect := httptest.NewServer(http.RedirectHandler("http://169.254.169.254/", http.StatusFound))
	defer redirect.Close()
	garbage := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>")) }))
	defer garbage.Close()
	c := newHTTPClient(time.Second)
	for name, u := range map[string]string{"redirect": redirect.URL, "garbage": garbage.URL, "refused": "http://127.0.0.1:1"} {
		if _, err := fetchPlugins(context.Background(), c, u); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
}

func TestCollectStatusAndSeries(t *testing.T) {
	db, repo, c := setup(t)
	ctx := context.Background()
	srv, _ := fakeFluentd(t, 50)
	n, err := repo.Create(ctx, nodes.Input{Name: "web-1", MonitorURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if s := c.Status(n.ID); s.State != StatePending {
		t.Fatalf("initial state = %s", s.State)
	}

	start := time.Unix(1_800_000_000, 0)
	for i := range 3 {
		c.now = func() time.Time { return start.Add(time.Duration(i*10) * time.Second) }
		if err := c.CollectOnce(ctx); err != nil {
			t.Fatal(err)
		}
	}
	s := c.Status(n.ID)
	if s.State != StateUp || s.Summary == nil || s.Summary.Plugins != 2 || s.Summary.BufferBytes != 1024 || s.Summary.RetryCount != 3 {
		t.Fatalf("status = %+v summary = %+v", s, s.Summary)
	}
	if s.Summary.EmitRecordsRate == nil || *s.Summary.EmitRecordsRate != 5 {
		t.Errorf("emit rate = %v, want 5/s", s.Summary.EmitRecordsRate)
	}

	series, err := Series(ctx, db, n.ID, start, start.Add(time.Minute), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(series) != 2 {
		t.Fatalf("series = %d", len(series))
	}
	es := series[1]
	if es.PluginID != "out_es" || len(es.TS) != 3 || es.EmitRecordsRate[0] != nil || *es.EmitRecordsRate[2] != 5 || *es.WriteRate[1] != 0.1 {
		t.Fatalf("out_es series = %+v", es)
	}

	c.now = func() time.Time { return start.Add(time.Minute) }
	srv.Close()
	if err := c.CollectOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if s := c.Status(n.ID); s.State != StateDown || s.LastError == "" || s.LastSeen == nil {
		t.Fatalf("after outage: %+v", s)
	}

	deleted, err := Prune(ctx, db, start.Add(15*time.Second))
	if err != nil || deleted != 4 {
		t.Errorf("prune deleted %d, %v", deleted, err)
	}
}

func TestStaleState(t *testing.T) {
	_, repo, c := setup(t)
	ctx := context.Background()
	srv, _ := fakeFluentd(t, 1)
	n, _ := repo.Create(ctx, nodes.Input{Name: "a", MonitorURL: srv.URL})
	now := time.Unix(1_800_000_000, 0)
	c.now = func() time.Time { return now }
	if err := c.CollectOnce(ctx); err != nil {
		t.Fatal(err)
	}
	c.now = func() time.Time { return now.Add(c.cfg.StaleAfter.D() + time.Second) }
	if s := c.Status(n.ID); s.State != StateStale {
		t.Fatalf("state = %s", s.State)
	}
}

func TestMultiWorker(t *testing.T) {
	_, repo, c := setup(t)
	ctx := context.Background()
	srv, calls := fakeFluentd(t, 1)
	// Worker 1 listens on port+1: start a second listener there if free.
	_, port, _ := net.SplitHostPort(srv.Listener.Addr().String())
	var p int
	fmt.Sscan(port, &p)
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p+1))
	if err != nil {
		t.Skip("port+1 not available")
	}
	second := &httptest.Server{Listener: ln, Config: &http.Server{Handler: srv.Config.Handler}}
	second.Start()
	defer second.Close()

	counts, err := c.Test(ctx, srv.URL, 2)
	if err != nil || len(counts) != 2 || counts[0] != 2 {
		t.Fatalf("Test = %v, %v", counts, err)
	}
	n, _ := repo.Create(ctx, nodes.Input{Name: "mw", MonitorURL: srv.URL, Workers: 2})
	calls.Store(0)
	if err := c.CollectOnce(ctx); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 || c.Status(n.ID).Summary.Plugins != 4 {
		t.Fatalf("calls=%d status=%+v", calls.Load(), c.Status(n.ID).Summary)
	}
	if _, err := c.Test(ctx, srv.URL, 3); err == nil || !strings.Contains(err.Error(), "worker 2") {
		t.Errorf("missing worker: %v", err)
	}
}
