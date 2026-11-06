// Package nodes stores the Fluentd instances monitored by flui.
package nodes

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const DefaultMonitorPort = 24220

var (
	ErrNotFound  = errors.New("node not found")
	ErrDuplicate = errors.New("a node with this name already exists")
	namePattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 ._-]{0,63}$`)
)

type Node struct {
	ID         int64     `json:"id"`
	Name       string    `json:"name"`
	Mode       string    `json:"mode"`
	MonitorURL string    `json:"monitor_url"`
	Workers    int       `json:"workers"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type Input struct {
	Name       string `json:"name"`
	MonitorURL string `json:"monitor_url"`
	Workers    int    `json:"workers"`
}

// ValidationError is returned for invalid user input; its message is safe to show.
type ValidationError struct{ Field, Msg string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Msg }

// Normalize validates the input and canonicalises the monitor URL to scheme://host:port.
func (in *Input) Normalize() error {
	in.Name = strings.TrimSpace(in.Name)
	if !namePattern.MatchString(in.Name) {
		return &ValidationError{"name", "use 1-64 letters, digits, spaces, '.', '_' or '-'"}
	}
	if in.Workers == 0 {
		in.Workers = 1
	}
	if in.Workers < 1 || in.Workers > 64 {
		return &ValidationError{"workers", "must be between 1 and 64"}
	}
	u, err := NormalizeMonitorURL(in.MonitorURL)
	if err != nil {
		return err
	}
	in.MonitorURL = u
	return nil
}

func NormalizeMonitorURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	bad := &ValidationError{"monitor_url", "must look like http://10.0.0.21:24220"}
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return "", bad
	}
	port := DefaultMonitorPort
	if p := u.Port(); p != "" {
		if port, err = strconv.Atoi(p); err != nil || port < 1 || port > 65535 {
			return "", bad
		}
	}
	return u.Scheme + "://" + net.JoinHostPort(u.Hostname(), strconv.Itoa(port)), nil
}

// WorkerURL returns the monitor_agent URL of a worker: Fluentd listens on port + worker_id in multi-worker mode.
func WorkerURL(base string, worker int) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		return "", fmt.Errorf("invalid monitor URL %q", base)
	}
	u.Host = net.JoinHostPort(u.Hostname(), strconv.Itoa(port+worker))
	return u.String(), nil
}

// Snippet returns the Fluentd configuration that exposes metrics for this node.
func Snippet(n Node) string {
	u, _ := url.Parse(n.MonitorURL)
	bind := u.Hostname()
	return fmt.Sprintf(`# flui: metrics for node %q
# Allow TCP %s only from the flui server. See docs/agentless.md.
<source>
  @type monitor_agent
  bind %s
  port %s
  include_config false
</source>
`, n.Name, portRange(u.Port(), n.Workers), bind, u.Port())
}

func portRange(port string, workers int) string {
	if workers <= 1 {
		return port
	}
	p, _ := strconv.Atoi(port)
	return fmt.Sprintf("%d-%d", p, p+workers-1)
}

type Repo struct{ db *sql.DB }

func NewRepo(db *sql.DB) *Repo { return &Repo{db: db} }

const nodeCols = `id, name, mode, monitor_url, workers, created_at, updated_at`

func scanNode(row interface{ Scan(...any) error }) (Node, error) {
	var n Node
	var created, updated int64
	err := row.Scan(&n.ID, &n.Name, &n.Mode, &n.MonitorURL, &n.Workers, &created, &updated)
	n.CreatedAt, n.UpdatedAt = time.Unix(created, 0).UTC(), time.Unix(updated, 0).UTC()
	return n, err
}

func (r *Repo) List(ctx context.Context) ([]Node, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+nodeCols+` FROM nodes ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Node{}
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *Repo) Get(ctx context.Context, id int64) (Node, error) {
	n, err := scanNode(r.db.QueryRowContext(ctx, `SELECT `+nodeCols+` FROM nodes WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return n, ErrNotFound
	}
	return n, err
}

func (r *Repo) Create(ctx context.Context, in Input) (Node, error) {
	if err := in.Normalize(); err != nil {
		return Node{}, err
	}
	now := time.Now().Unix()
	n, err := scanNode(r.db.QueryRowContext(ctx, `INSERT INTO nodes (name, monitor_url, workers, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?) RETURNING `+nodeCols, in.Name, in.MonitorURL, in.Workers, now, now))
	return n, mapErr(err)
}

func (r *Repo) Update(ctx context.Context, id int64, in Input) (Node, error) {
	if err := in.Normalize(); err != nil {
		return Node{}, err
	}
	n, err := scanNode(r.db.QueryRowContext(ctx, `UPDATE nodes SET name = ?, monitor_url = ?, workers = ?, updated_at = ?
		WHERE id = ? RETURNING `+nodeCols, in.Name, in.MonitorURL, in.Workers, time.Now().Unix(), id))
	return n, mapErr(err)
}

func (r *Repo) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func mapErr(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return ErrNotFound
	case strings.Contains(err.Error(), "UNIQUE constraint failed"):
		return ErrDuplicate
	}
	return err
}
