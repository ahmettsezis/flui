// Package auth implements local users, password hashing, sessions, roles and the audit log.
package auth

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ahmettsezis/flui/internal/config"
)

type Role string

const (
	RoleViewer   Role = "viewer"
	RoleOperator Role = "operator"
	RoleAdmin    Role = "admin"
)

var roleRank = map[Role]int{RoleViewer: 1, RoleOperator: 2, RoleAdmin: 3}

// Allows reports whether r has at least the privileges of required.
func (r Role) Allows(required Role) bool {
	return roleRank[r] >= roleRank[required] && roleRank[required] > 0
}

var (
	ErrInvalidCredentials = errors.New("invalid username or password, or the account is temporarily locked")
	ErrNoSession          = errors.New("not signed in")
	ErrWeakPassword       = errors.New("password does not meet the policy")
)

type User struct {
	ID                 int64  `json:"id"`
	Username           string `json:"username"`
	Role               Role   `json:"role"`
	MustChangePassword bool   `json:"must_change_password"`
}

type Session struct {
	IDHash    []byte
	CSRFToken string
}

type Service struct {
	db  *sql.DB
	cfg config.Auth
	now func() time.Time
	// dummyHash is verified for unknown users so response times do not reveal which usernames exist.
	dummyHash string
}

func NewService(db *sql.DB, cfg config.Auth) (*Service, error) {
	dummy, err := HashPassword(randomToken())
	if err != nil {
		return nil, err
	}
	return &Service{db: db, cfg: cfg, now: time.Now, dummyHash: dummy}, nil
}

func hashSessionID(raw string) []byte {
	sum := sha256.Sum256([]byte(raw))
	return sum[:]
}

// Bootstrap creates the initial admin with a random password if no user exists yet.
func (s *Service) Bootstrap(ctx context.Context) (password string, created bool, err error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil || n > 0 {
		return "", false, err
	}
	password = GeneratePassword(20)
	if _, err := s.createUser(ctx, "admin", password, RoleAdmin, true); err != nil {
		return "", false, err
	}
	s.Audit(ctx, AuditEntry{Username: "system", Action: "user.bootstrap", Target: "admin"})
	return password, true, nil
}

func (s *Service) createUser(ctx context.Context, username, password string, role Role, mustChange bool) (int64, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return 0, err
	}
	now := s.now().Unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO users (username, password_hash, role, must_change_password, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`, username, hash, string(role), mustChange, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// Login verifies credentials, applies lockout rules and creates a session. It returns the raw session ID.
func (s *Service) Login(ctx context.Context, username, password, ip, userAgent string) (string, *Session, *User, error) {
	var (
		u           User
		hash        string
		failed      int
		lockedUntil int64
	)
	err := s.db.QueryRowContext(ctx, `SELECT id, username, role, must_change_password, password_hash, failed_logins, locked_until
		FROM users WHERE username = ?`, username).Scan(&u.ID, &u.Username, &u.Role, &u.MustChangePassword, &hash, &failed, &lockedUntil)
	if errors.Is(err, sql.ErrNoRows) {
		_, _ = VerifyPassword(password, s.dummyHash)
		s.Audit(ctx, AuditEntry{Username: username, Action: "auth.login_failed", IP: ip, Details: map[string]any{"reason": "unknown user"}})
		return "", nil, nil, ErrInvalidCredentials
	}
	if err != nil {
		return "", nil, nil, err
	}
	ok, err := VerifyPassword(password, hash)
	if err != nil {
		return "", nil, nil, err
	}
	now := s.now()
	if lockedUntil > now.Unix() {
		s.Audit(ctx, AuditEntry{UserID: u.ID, Username: u.Username, Action: "auth.login_failed", IP: ip, Details: map[string]any{"reason": "locked"}})
		return "", nil, nil, ErrInvalidCredentials
	}
	if !ok {
		failed++
		reason := "bad password"
		if failed >= s.cfg.LockoutThreshold {
			lockedUntil, failed, reason = now.Add(s.cfg.LockoutDuration.D()).Unix(), 0, "bad password, account locked"
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE users SET failed_logins = ?, locked_until = ? WHERE id = ?`, failed, lockedUntil, u.ID); err != nil {
			return "", nil, nil, err
		}
		s.Audit(ctx, AuditEntry{UserID: u.ID, Username: u.Username, Action: "auth.login_failed", IP: ip, Details: map[string]any{"reason": reason}})
		return "", nil, nil, ErrInvalidCredentials
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE users SET failed_logins = 0, locked_until = 0 WHERE id = ?`, u.ID); err != nil {
		return "", nil, nil, err
	}
	raw, sess, err := s.newSession(ctx, u.ID, ip, userAgent)
	if err != nil {
		return "", nil, nil, err
	}
	s.Audit(ctx, AuditEntry{UserID: u.ID, Username: u.Username, Action: "auth.login", IP: ip})
	return raw, sess, &u, nil
}

func (s *Service) newSession(ctx context.Context, userID int64, ip, userAgent string) (string, *Session, error) {
	raw := randomToken()
	now := s.now()
	sess := &Session{IDHash: hashSessionID(raw), CSRFToken: randomToken()}
	if len(userAgent) > 256 {
		userAgent = userAgent[:256]
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO sessions (id_hash, user_id, csrf_token, created_at, last_seen_at, expires_at, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, sess.IDHash, userID, sess.CSRFToken, now.Unix(), now.Unix(),
		now.Add(s.cfg.SessionMaxLifetime.D()).Unix(), ip, strings.ToValidUTF8(userAgent, ""))
	return raw, sess, err
}

// Authenticate resolves a raw session ID, enforcing idle and absolute timeouts.
func (s *Service) Authenticate(ctx context.Context, raw string) (*Session, *User, error) {
	if raw == "" {
		return nil, nil, ErrNoSession
	}
	idHash := hashSessionID(raw)
	var (
		sess      = Session{IDHash: idHash}
		u         User
		lastSeen  int64
		expiresAt int64
	)
	err := s.db.QueryRowContext(ctx, `SELECT s.csrf_token, s.last_seen_at, s.expires_at, u.id, u.username, u.role, u.must_change_password
		FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.id_hash = ?`, idHash).
		Scan(&sess.CSRFToken, &lastSeen, &expiresAt, &u.ID, &u.Username, &u.Role, &u.MustChangePassword)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNoSession
	}
	if err != nil {
		return nil, nil, err
	}
	now := s.now()
	if now.Unix() >= expiresAt || now.Sub(time.Unix(lastSeen, 0)) >= s.cfg.SessionIdleTimeout.D() {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, idHash)
		return nil, nil, ErrNoSession
	}
	if now.Unix()-lastSeen >= 30 {
		if _, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE id_hash = ?`, now.Unix(), idHash); err != nil {
			return nil, nil, err
		}
	}
	return &sess, &u, nil
}

func (s *Service) Logout(ctx context.Context, sess *Session, u *User, ip string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, sess.IDHash)
	s.Audit(ctx, AuditEntry{UserID: u.ID, Username: u.Username, Action: "auth.logout", IP: ip})
	return err
}

func (s *Service) checkPolicy(username, password string) error {
	n := utf8.RuneCountInString(password)
	switch {
	case n < s.cfg.PasswordMinLength:
		return fmt.Errorf("%w: use at least %d characters", ErrWeakPassword, s.cfg.PasswordMinLength)
	case len(password) > 256:
		return fmt.Errorf("%w: use at most 256 bytes", ErrWeakPassword)
	case strings.EqualFold(password, username):
		return fmt.Errorf("%w: must differ from the username", ErrWeakPassword)
	}
	return nil
}

// ChangePassword verifies the current password, sets a new one and signs out all other sessions of the user.
func (s *Service) ChangePassword(ctx context.Context, u *User, keep *Session, current, newPassword, ip string) error {
	var hash string
	if err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM users WHERE id = ?`, u.ID).Scan(&hash); err != nil {
		return err
	}
	ok, err := VerifyPassword(current, hash)
	if err != nil {
		return err
	}
	if !ok {
		s.Audit(ctx, AuditEntry{UserID: u.ID, Username: u.Username, Action: "auth.password_change_failed", IP: ip})
		return ErrInvalidCredentials
	}
	if err := s.checkPolicy(u.Username, newPassword); err != nil {
		return err
	}
	if current == newPassword {
		return fmt.Errorf("%w: must differ from the current password", ErrWeakPassword)
	}
	newHash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after commit
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, must_change_password = 0, updated_at = ? WHERE id = ?`,
		newHash, s.now().Unix(), u.ID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id_hash != ?`, u.ID, keep.IDHash); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.Audit(ctx, AuditEntry{UserID: u.ID, Username: u.Username, Action: "auth.password_change", IP: ip})
	return nil
}

// ResetPassword sets a new random password that must be changed at next sign-in, unlocks the account and signs
// out all sessions of the user. Used by the `flui-server user reset-password` command.
func (s *Service) ResetPassword(ctx context.Context, username string) (string, error) {
	password := GeneratePassword(20)
	hash, err := HashPassword(password)
	if err != nil {
		return "", err
	}
	var id int64
	err = s.db.QueryRowContext(ctx, `UPDATE users SET password_hash = ?, must_change_password = 1, failed_logins = 0, locked_until = 0,
		updated_at = ? WHERE username = ? RETURNING id`, hash, s.now().Unix(), username).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("user %q does not exist", username)
	}
	if err != nil {
		return "", err
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
		return "", err
	}
	s.Audit(ctx, AuditEntry{Username: "cli", Action: "auth.password_reset", Target: username})
	return password, nil
}

// Cleanup removes expired sessions.
func (s *Service) Cleanup(ctx context.Context) error {
	now := s.now()
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ? OR last_seen_at <= ?`,
		now.Unix(), now.Add(-s.cfg.SessionIdleTimeout.D()).Unix())
	return err
}

type AuditEntry struct {
	UserID   int64
	Username string
	Action   string
	Target   string
	IP       string
	Details  map[string]any
}

// Audit records a security-relevant event. Failures are not returned to avoid breaking the calling operation,
// but the entry is written synchronously.
func (s *Service) Audit(ctx context.Context, e AuditEntry) {
	details := "{}"
	if len(e.Details) > 0 {
		if b, err := json.Marshal(e.Details); err == nil {
			details = string(b)
		}
	}
	if len(e.Username) > 64 {
		e.Username = e.Username[:64]
	}
	e.Username = strings.ToValidUTF8(e.Username, "")
	var uid any
	if e.UserID != 0 {
		uid = e.UserID
	}
	_, _ = s.db.ExecContext(context.WithoutCancel(ctx), `INSERT INTO audit_log (ts, user_id, username, action, target, ip, details) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		s.now().Unix(), uid, e.Username, e.Action, e.Target, e.IP, details)
}
