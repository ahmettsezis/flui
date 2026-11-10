package server

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/ahmettsezis/flui/internal/auth"
	"github.com/ahmettsezis/flui/internal/collector"
	"github.com/ahmettsezis/flui/internal/config"
)

const (
	jobInterval    = time.Hour
	auditRetention = 365 * 24 * time.Hour
)

func InitialPasswordFile(cfg config.Config) string {
	return filepath.Join(cfg.DataDir, "initial-admin-password")
}

// bootstrapAdmin creates the first admin user and writes its password to a 0600 file and, once, to the log.
func bootstrapAdmin(ctx context.Context, svc *auth.Service, path string, log *slog.Logger) error {
	pw, created, err := svc.Bootstrap(ctx)
	if err != nil || !created {
		return err
	}
	if err := os.WriteFile(path, []byte(pw+"\n"), 0o600); err != nil {
		return err
	}
	log.Warn("initial admin password created; sign in as admin and change it",
		"username", "admin", "password", pw, "file", path)
	return nil
}

// runJobs applies retention and cleans up expired sessions every hour.
func runJobs(ctx context.Context, cfg config.Config, db *sql.DB, svc *auth.Service, log *slog.Logger) {
	t := time.NewTicker(jobInterval)
	defer t.Stop()
	for {
		now := time.Now()
		if n, err := collector.Prune(ctx, db, now.Add(-cfg.Retention.D())); err != nil {
			log.Error("pruning metrics failed", "err", err)
		} else if n > 0 {
			log.Debug("pruned old metrics", "rows", n)
		}
		if err := svc.Cleanup(ctx); err != nil {
			log.Error("cleaning up sessions failed", "err", err)
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM audit_log WHERE ts < ?`, now.Add(-auditRetention).Unix()); err != nil {
			log.Error("pruning audit log failed", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
