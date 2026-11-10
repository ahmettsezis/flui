package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/ahmettsezis/flui/internal/auth"
	"github.com/ahmettsezis/flui/internal/collector"
	"github.com/ahmettsezis/flui/internal/nodes"
)

type nodeResponse struct {
	nodes.Node
	Status  collector.Status `json:"status"`
	Snippet string           `json:"snippet,omitempty"`
}

func (s *server) listNodes(w http.ResponseWriter, r *http.Request) {
	list, err := s.Nodes.List(r.Context())
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	out := make([]nodeResponse, 0, len(list))
	for _, n := range list {
		out = append(out, nodeResponse{Node: n, Status: s.Collector.Status(n.ID)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *server) nodeID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusNotFound, "node not found")
		return 0, false
	}
	return id, true
}

func (s *server) getNode(w http.ResponseWriter, r *http.Request) {
	id, ok := s.nodeID(w, r)
	if !ok {
		return
	}
	n, err := s.Nodes.Get(r.Context(), id)
	if s.nodeErr(w, r, err) {
		return
	}
	writeJSON(w, http.StatusOK, nodeResponse{Node: n, Status: s.Collector.Status(n.ID), Snippet: nodes.Snippet(n)})
}

func (s *server) createNode(w http.ResponseWriter, r *http.Request) {
	var in nodes.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	n, err := s.Nodes.Create(r.Context(), in)
	if s.nodeErr(w, r, err) {
		return
	}
	s.audit(r, "node.create", n)
	s.Collector.Trigger()
	writeJSON(w, http.StatusCreated, nodeResponse{Node: n, Status: s.Collector.Status(n.ID), Snippet: nodes.Snippet(n)})
}

func (s *server) updateNode(w http.ResponseWriter, r *http.Request) {
	id, ok := s.nodeID(w, r)
	if !ok {
		return
	}
	var in nodes.Input
	if !decodeJSON(w, r, &in) {
		return
	}
	n, err := s.Nodes.Update(r.Context(), id, in)
	if s.nodeErr(w, r, err) {
		return
	}
	s.audit(r, "node.update", n)
	s.Collector.Forget(n.ID)
	s.Collector.Trigger()
	writeJSON(w, http.StatusOK, nodeResponse{Node: n, Status: s.Collector.Status(n.ID), Snippet: nodes.Snippet(n)})
}

func (s *server) deleteNode(w http.ResponseWriter, r *http.Request) {
	id, ok := s.nodeID(w, r)
	if !ok {
		return
	}
	n, err := s.Nodes.Get(r.Context(), id)
	if s.nodeErr(w, r, err) {
		return
	}
	if s.nodeErr(w, r, s.Nodes.Delete(r.Context(), id)) {
		return
	}
	s.audit(r, "node.delete", n)
	s.Collector.Forget(id)
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) testNode(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MonitorURL string `json:"monitor_url"`
		Workers    int    `json:"workers"`
	}
	if !decodeJSON(w, r, &in) {
		return
	}
	probe := nodes.Input{Name: "test", MonitorURL: in.MonitorURL, Workers: in.Workers}
	if s.nodeErr(w, r, probe.Normalize()) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(probe.Workers)*s.Config.Collector.Timeout.D())
	defer cancel()
	counts, err := s.Collector.Test(ctx, probe.MonitorURL, probe.Workers)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error(), "monitor_url": probe.MonitorURL})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "output_plugins": counts, "monitor_url": probe.MonitorURL})
}

// rangeOptions are the selectable chart ranges.
var rangeOptions = map[string]time.Duration{
	"15m": 15 * time.Minute, "1h": time.Hour, "6h": 6 * time.Hour, "24h": 24 * time.Hour, "2d": 48 * time.Hour, "7d": 7 * 24 * time.Hour,
}

const maxPoints = 360

func (s *server) nodeMetrics(w http.ResponseWriter, r *http.Request) {
	id, ok := s.nodeID(w, r)
	if !ok {
		return
	}
	if _, err := s.Nodes.Get(r.Context(), id); s.nodeErr(w, r, err) {
		return
	}
	rangeParam := r.URL.Query().Get("range")
	if rangeParam == "" {
		rangeParam = "1h"
	}
	span, ok := rangeOptions[rangeParam]
	if !ok {
		writeError(w, http.StatusBadRequest, "range must be one of 15m, 1h, 6h, 24h, 2d, 7d")
		return
	}
	if maxSpan := s.Config.Retention.D(); span > maxSpan {
		span = maxSpan
	}
	to := time.Now()
	from := to.Add(-span)
	step := max(int64(s.Config.Collector.Interval.D().Seconds()), int64(span.Seconds())/maxPoints)
	series, err := collector.Series(r.Context(), s.SQL, id, from, to, step)
	if err != nil {
		s.internalError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"from": from.Unix(), "to": to.Unix(), "step": step, "plugins": series})
}

func (s *server) nodeErr(w http.ResponseWriter, r *http.Request, err error) bool {
	var verr *nodes.ValidationError
	switch {
	case err == nil:
		return false
	case errors.As(err, &verr):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": verr.Msg, "field": verr.Field})
	case errors.Is(err, nodes.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, nodes.ErrDuplicate):
		writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error(), "field": "name"})
	default:
		s.internalError(w, r, err)
	}
	return true
}

func (s *server) audit(r *http.Request, action string, n nodes.Node) {
	u := userFrom(r)
	s.Auth.Audit(r.Context(), auth.AuditEntry{
		UserID: u.ID, Username: u.Username, Action: action, Target: n.Name, IP: s.clientIP(r),
		Details: map[string]any{"id": n.ID, "monitor_url": n.MonitorURL, "workers": n.Workers},
	})
}
