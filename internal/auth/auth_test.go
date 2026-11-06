package auth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/ahmettsezis/flui/internal/config"
	"github.com/ahmettsezis/flui/internal/store"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func newService(t *testing.T) (*Service, *clock) {
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
	s, err := NewService(st.DB, config.Default().Auth)
	if err != nil {
		t.Fatal(err)
	}
	c := &clock{t: time.Unix(1_800_000_000, 0)}
	s.now = c.now
	return s, c
}

func auditCount(t *testing.T, s *Service, action string) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE action = ?`, action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPasswordHashing(t *testing.T) {
	h, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$m=65536,t=3,p=4$") {
		t.Fatalf("hash = %s", h)
	}
	if ok, err := VerifyPassword("correct horse battery staple", h); !ok || err != nil {
		t.Errorf("verify correct: %v %v", ok, err)
	}
	if ok, _ := VerifyPassword("wrong", h); ok {
		t.Error("wrong password accepted")
	}
	for _, bad := range []string{"", "$argon2i$v=19$m=1,t=1,p=1$a$b", "$argon2id$v=19$m=999999999,t=1,p=1$YQ$YQ"} {
		if _, err := VerifyPassword("x", bad); err == nil {
			t.Errorf("malformed hash %q accepted", bad)
		}
	}
	if p := GeneratePassword(20); len(p) != 20 || strings.ContainsAny(p, "0O1lI") {
		t.Errorf("generated password %q", p)
	}
}

func TestBootstrapAndLogin(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	pw, created, err := s.Bootstrap(ctx)
	if err != nil || !created || len(pw) != 20 {
		t.Fatalf("bootstrap: %q %v %v", pw, created, err)
	}
	if _, created, _ := s.Bootstrap(ctx); created {
		t.Fatal("bootstrap ran twice")
	}

	raw, sess, u, err := s.Login(ctx, "ADMIN", pw, "10.0.0.1", "test")
	if err != nil {
		t.Fatal(err)
	}
	if u.Role != RoleAdmin || !u.MustChangePassword || sess.CSRFToken == "" {
		t.Fatalf("user = %+v", u)
	}
	var stored []byte
	if err := s.db.QueryRow(`SELECT id_hash FROM sessions`).Scan(&stored); err != nil || string(stored) == raw {
		t.Fatal("raw session ID must not be stored")
	}
	if _, au, err := s.Authenticate(ctx, raw); err != nil || au.ID != u.ID {
		t.Fatalf("authenticate: %v", err)
	}
	if _, _, err := s.Authenticate(ctx, "forged"); !errors.Is(err, ErrNoSession) {
		t.Errorf("forged session: %v", err)
	}
	if auditCount(t, s, "auth.login") != 1 || auditCount(t, s, "user.bootstrap") != 1 {
		t.Error("audit entries missing")
	}
}

func TestLockout(t *testing.T) {
	s, c := newService(t)
	ctx := context.Background()
	pw, _, _ := s.Bootstrap(ctx)
	for range s.cfg.LockoutThreshold {
		if _, _, _, err := s.Login(ctx, "admin", "wrong", "ip", ""); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatal(err)
		}
	}
	if _, _, _, err := s.Login(ctx, "admin", pw, "ip", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatal("locked account accepted correct password")
	}
	c.t = c.t.Add(s.cfg.LockoutDuration.D() + time.Second)
	if _, _, _, err := s.Login(ctx, "admin", pw, "ip", ""); err != nil {
		t.Fatalf("after lockout: %v", err)
	}
	if _, _, _, err := s.Login(ctx, "nobody", "x", "ip", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("unknown user: %v", err)
	}
}

func TestSessionTimeouts(t *testing.T) {
	s, c := newService(t)
	ctx := context.Background()
	pw, _, _ := s.Bootstrap(ctx)

	raw, _, _, _ := s.Login(ctx, "admin", pw, "ip", "")
	c.t = c.t.Add(s.cfg.SessionIdleTimeout.D())
	if _, _, err := s.Authenticate(ctx, raw); !errors.Is(err, ErrNoSession) {
		t.Errorf("idle session still valid: %v", err)
	}

	raw, _, _, _ = s.Login(ctx, "admin", pw, "ip", "")
	for elapsed := time.Duration(0); elapsed < s.cfg.SessionMaxLifetime.D(); elapsed += 10 * time.Minute {
		c.t = c.t.Add(10 * time.Minute)
		_, _, _ = s.Authenticate(ctx, raw)
	}
	if _, _, err := s.Authenticate(ctx, raw); !errors.Is(err, ErrNoSession) {
		t.Errorf("session past max lifetime still valid: %v", err)
	}
}

func TestChangeAndResetPassword(t *testing.T) {
	s, _ := newService(t)
	ctx := context.Background()
	pw, _, _ := s.Bootstrap(ctx)
	rawA, sessA, u, _ := s.Login(ctx, "admin", pw, "ip", "")
	rawB, _, _, _ := s.Login(ctx, "admin", pw, "ip", "")

	if err := s.ChangePassword(ctx, u, sessA, pw, "short", "ip"); !errors.Is(err, ErrWeakPassword) {
		t.Errorf("weak password: %v", err)
	}
	if err := s.ChangePassword(ctx, u, sessA, "wrong-current", "a-much-better-password", "ip"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("wrong current: %v", err)
	}
	if err := s.ChangePassword(ctx, u, sessA, pw, "a-much-better-password", "ip"); err != nil {
		t.Fatal(err)
	}
	if _, au, err := s.Authenticate(ctx, rawA); err != nil || au.MustChangePassword {
		t.Errorf("current session: %v %+v", err, au)
	}
	if _, _, err := s.Authenticate(ctx, rawB); !errors.Is(err, ErrNoSession) {
		t.Error("other sessions must be signed out after a password change")
	}

	newPw, err := s.ResetPassword(ctx, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Authenticate(ctx, rawA); !errors.Is(err, ErrNoSession) {
		t.Error("reset must sign out all sessions")
	}
	if _, _, u, err := s.Login(ctx, "admin", newPw, "ip", ""); err != nil || !u.MustChangePassword {
		t.Errorf("login after reset: %v", err)
	}
	if _, err := s.ResetPassword(ctx, "ghost"); err == nil {
		t.Error("reset of unknown user succeeded")
	}
}

func TestRoles(t *testing.T) {
	if !RoleAdmin.Allows(RoleOperator) || !RoleOperator.Allows(RoleViewer) || RoleViewer.Allows(RoleOperator) || Role("x").Allows(RoleViewer) || RoleAdmin.Allows(Role("")) {
		t.Error("role ordering wrong")
	}
}

func TestRateLimiter(t *testing.T) {
	r := NewRateLimiter(2, time.Minute)
	c := &clock{t: time.Unix(0, 0)}
	r.now = c.now
	got := []bool{r.Allow("a"), r.Allow("a"), r.Allow("a"), r.Allow("b")}
	for i, want := range []bool{true, true, false, true} {
		if got[i] != want {
			t.Fatalf("attempt %d = %v, want %v", i, got[i], want)
		}
	}
	c.t = c.t.Add(time.Minute)
	if !r.Allow("a") {
		t.Fatal("window did not reset")
	}
}
