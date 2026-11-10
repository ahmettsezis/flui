package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "data")
	s, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, dir
}

func TestOpenCreatesPrivateDataDir(t *testing.T) {
	_, dir := open(t)
	st, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o700 {
		t.Errorf("data dir mode = %v", st.Mode().Perm())
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s, _ := open(t)
	ctx := context.Background()
	applied, err := s.Migrate(ctx)
	if err != nil || len(applied) == 0 {
		t.Fatalf("first migrate: applied=%v err=%v", applied, err)
	}
	applied, err = s.Migrate(ctx)
	if err != nil || len(applied) != 0 {
		t.Fatalf("second migrate: applied=%v err=%v", applied, err)
	}
	var mode string
	if err := s.DB.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&mode); err != nil || mode != "wal" {
		t.Errorf("journal_mode = %q, err = %v", mode, err)
	}
	var fk int
	if err := s.DB.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys = %d, err = %v", fk, err)
	}
}

func TestLoadMigrations(t *testing.T) {
	ms, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	for i, m := range ms {
		if m.version != i+1 {
			t.Errorf("migration %s has version %d, want %d (no gaps)", m.name, m.version, i+1)
		}
	}
}
