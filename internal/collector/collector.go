// Package collector polls Fluentd monitor_agent endpoints and stores the samples.
package collector

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/ahmettsezis/flui/internal/config"
	"github.com/ahmettsezis/flui/internal/nodes"
)

const (
	StatePending = "pending"
	StateUp      = "up"
	StateDown    = "down"
	StateStale   = "stale"

	maxParallel = 16
)

type Status struct {
	State     string     `json:"state"`
	LastSeen  *time.Time `json:"last_seen,omitempty"`
	LastError string     `json:"last_error,omitempty"`
	Summary   *Summary   `json:"summary,omitempty"`
}

// Summary aggregates the latest sample of all output plugins of a node.
type Summary struct {
	Plugins           int      `json:"plugins"`
	BufferBytes       int64    `json:"buffer_bytes"`
	QueueLength       int64    `json:"queue_length"`
	RetryCount        int64    `json:"retry_count"`
	MinAvailableRatio *float64 `json:"min_available_ratio,omitempty"`
	EmitRecordsRate   *float64 `json:"emit_records_rate,omitempty"`
	WriteRate         *float64 `json:"write_rate,omitempty"`
}

type snapshot struct {
	ts      time.Time
	plugins map[string]PluginMetrics // key: worker/plugin_id
}

type nodeState struct {
	lastAttempt time.Time
	lastSuccess time.Time
	lastErr     string
	cur, prev   *snapshot
}

type Collector struct {
	db      *sql.DB
	nodes   *nodes.Repo
	log     *slog.Logger
	cfg     config.Collector
	client  *http.Client
	now     func() time.Time
	trigger chan struct{}

	mu     sync.RWMutex
	states map[int64]*nodeState
}

func New(db *sql.DB, repo *nodes.Repo, cfg config.Collector, log *slog.Logger) *Collector {
	return &Collector{
		db: db, nodes: repo, log: log, cfg: cfg,
		client:  newHTTPClient(cfg.Timeout.D()),
		now:     time.Now,
		trigger: make(chan struct{}, 1),
		states:  map[int64]*nodeState{},
	}
}

// Run collects every cfg.Interval (and on Trigger) until ctx is cancelled.
func (c *Collector) Run(ctx context.Context) {
	t := time.NewTicker(c.cfg.Interval.D())
	defer t.Stop()
	for {
		if err := c.CollectOnce(ctx); err != nil && ctx.Err() == nil {
			c.log.Error("collection failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-c.trigger:
		}
	}
}

// Trigger requests an immediate collection, e.g. after a node was added.
func (c *Collector) Trigger() {
	select {
	case c.trigger <- struct{}{}:
	default:
	}
}

func (c *Collector) Forget(id int64) {
	c.mu.Lock()
	delete(c.states, id)
	c.mu.Unlock()
}

type result struct {
	node    nodes.Node
	ts      time.Time
	workers map[int][]PluginMetrics
	err     error
}

func (c *Collector) CollectOnce(ctx context.Context) error {
	list, err := c.nodes.List(ctx)
	if err != nil {
		return err
	}
	results := make([]result, len(list))
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	for i, n := range list {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = c.collectNode(ctx, n)
		}()
	}
	wg.Wait()

	tx, err := c.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	for _, r := range results {
		for w, ps := range r.workers {
			if err := insertSamples(ctx, tx, r.node.ID, r.ts, w, ps); err != nil {
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	for _, r := range results {
		if r.node.ID == 0 {
			continue
		}
		st := c.states[r.node.ID]
		if st == nil {
			st = &nodeState{}
			c.states[r.node.ID] = st
		}
		st.lastAttempt = r.ts
		if r.err != nil {
			if st.lastErr != r.err.Error() {
				c.log.Warn("node unreachable", "node", r.node.Name, "err", r.err)
			}
			st.lastErr = r.err.Error()
			continue
		}
		if st.lastErr != "" {
			c.log.Info("node reachable again", "node", r.node.Name)
		}
		st.lastErr, st.lastSuccess = "", r.ts
		snap := &snapshot{ts: r.ts, plugins: map[string]PluginMetrics{}}
		for w, ps := range r.workers {
			for _, p := range ps {
				snap.plugins[fmt.Sprintf("%d/%s", w, p.PluginID)] = p
			}
		}
		st.prev, st.cur = st.cur, snap
	}
	return nil
}

func (c *Collector) collectNode(ctx context.Context, n nodes.Node) result {
	r := result{node: n, ts: c.now(), workers: map[int][]PluginMetrics{}}
	for w := range n.Workers {
		u, err := nodes.WorkerURL(n.MonitorURL, w)
		if err == nil {
			var ps []PluginMetrics
			if ps, err = fetchPlugins(ctx, c.client, u); err == nil {
				r.workers[w] = ps
				continue
			}
		}
		if n.Workers > 1 {
			err = fmt.Errorf("worker %d: %w", w, err)
		}
		r.err = err
	}
	return r
}

// Test checks that a monitor_agent is reachable and returns the number of output plugins per worker.
func (c *Collector) Test(ctx context.Context, monitorURL string, workers int) ([]int, error) {
	counts := make([]int, 0, workers)
	for w := range workers {
		u, err := nodes.WorkerURL(monitorURL, w)
		if err != nil {
			return nil, err
		}
		ps, err := fetchPlugins(ctx, c.client, u)
		if err != nil {
			if workers > 1 {
				return counts, fmt.Errorf("worker %d (%s): %w", w, u, err)
			}
			return counts, err
		}
		counts = append(counts, len(ps))
	}
	return counts, nil
}

func (c *Collector) Status(id int64) Status {
	c.mu.RLock()
	defer c.mu.RUnlock()
	st := c.states[id]
	if st == nil {
		return Status{State: StatePending}
	}
	out := Status{LastError: st.lastErr}
	if !st.lastSuccess.IsZero() {
		ls := st.lastSuccess.UTC()
		out.LastSeen = &ls
	}
	switch {
	case st.lastErr != "":
		out.State = StateDown
	case c.now().Sub(st.lastSuccess) > c.cfg.StaleAfter.D():
		out.State = StateStale
	default:
		out.State = StateUp
	}
	if st.cur != nil {
		out.Summary = summarize(st.cur, st.prev)
	}
	return out
}

func summarize(cur, prev *snapshot) *Summary {
	s := &Summary{Plugins: len(cur.plugins)}
	var emitRate, writeRate float64
	haveRate := false
	for key, p := range cur.plugins {
		s.BufferBytes += i64(p.BufferTotalQueuedSize)
		s.QueueLength += i64(p.BufferQueueLength)
		s.RetryCount += i64(p.RetryCount)
		if r := p.BufferAvailableRatio; r != nil && (s.MinAvailableRatio == nil || *r < *s.MinAvailableRatio) {
			v := *r
			s.MinAvailableRatio = &v
		}
		if prev == nil {
			continue
		}
		dt := cur.ts.Sub(prev.ts).Seconds()
		old, ok := prev.plugins[key]
		if !ok || dt <= 0 {
			continue
		}
		if d, ok := delta(p.EmitRecords, old.EmitRecords); ok {
			emitRate += d / dt
			haveRate = true
		}
		if d, ok := delta(p.WriteCount, old.WriteCount); ok {
			writeRate += d / dt
			haveRate = true
		}
	}
	if haveRate {
		s.EmitRecordsRate, s.WriteRate = &emitRate, &writeRate
	}
	return s
}

func delta(cur, prev *float64) (float64, bool) {
	if cur == nil || prev == nil || *cur < *prev {
		return 0, false
	}
	return *cur - *prev, true
}

func i64(v *float64) int64 {
	if v == nil {
		return 0
	}
	return int64(*v)
}
