package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "flui.yaml")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDefaultsAreValid(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "missing.yaml"), false, nil)
	if err != nil {
		t.Fatalf("defaults should be valid: %v", err)
	}
	if cfg.MasterKeyFile != "/var/lib/flui/master.key" {
		t.Errorf("master key default = %q", cfg.MasterKeyFile)
	}
}

func TestMissingRequiredFile(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml"), true, nil); err == nil {
		t.Fatal("expected error for missing required file")
	}
}

func TestEmptyFile(t *testing.T) {
	if _, err := Load(writeFile(t, ""), true, nil); err != nil {
		t.Fatal(err)
	}
}

func TestFileAndEnvPrecedence(t *testing.T) {
	p := writeFile(t, `
server:
  listen: "127.0.0.1:9443"
  public_url: "https://flui.example.com:9443"
data_dir: /srv/flui
retention:
  raw: 1d
  logs: 14d
`)
	cfg, err := Load(p, true, []string{
		"FLUI_SERVER_LISTEN=:10443",
		"FLUI_AUTH_REQUIRE_TOTP_FOR_ADMINS=true",
		"FLUI_SERVER_TRUSTED_PROXIES=10.0.0.0/8, 192.168.0.0/16",
		"FLUI_RETENTION_HOUR=90d",
		"OTHER_VAR=ignored",
	})
	if err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name      string
		got, want any
	}{
		{"listen (env wins)", cfg.Server.Listen, ":10443"},
		{"public_url (file)", cfg.Server.PublicURL, "https://flui.example.com:9443"},
		{"data_dir", cfg.DataDir, "/srv/flui"},
		{"master key follows data_dir", cfg.MasterKeyFile, "/srv/flui/master.key"},
		{"totp", cfg.Auth.RequireTOTPForAdmins, true},
		{"proxies", strings.Join(cfg.Server.TrustedProxies, ","), "10.0.0.0/8,192.168.0.0/16"},
		{"raw", cfg.Retention.Raw.D(), 24 * time.Hour},
		{"logs", cfg.Retention.Logs.D(), 14 * Day},
		{"hour (env)", cfg.Retention.Hour.D(), 90 * Day},
		{"minute (default)", cfg.Retention.Minute.D(), 30 * Day},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

func TestInvalidConfigs(t *testing.T) {
	cases := map[string]struct {
		yaml string
		env  []string
		want string
	}{
		"unknown field":        {yaml: "server:\n  lisen: ':1'\n", want: "lisen"},
		"retention over 1y":    {yaml: "retention:\n  hour: 400d\n", want: "retention.hour"},
		"cert without key":     {yaml: "server:\n  tls:\n    cert_file: /x.crt\n", want: "cert_file and key_file"},
		"http public url":      {yaml: "server:\n  public_url: http://x\n", want: "public_url"},
		"bad duration":         {yaml: "collector:\n  interval: 10x\n", want: "invalid duration"},
		"bad env bool":         {env: []string{"FLUI_AUTH_REQUIRE_TOTP_FOR_ADMINS=maybe"}, want: "invalid boolean"},
		"bad proxy":            {env: []string{"FLUI_SERVER_TRUSTED_PROXIES=10.0.0.1"}, want: "not a CIDR"},
		"timeout >= interval":  {yaml: "collector:\n  interval: 5s\n  timeout: 5s\n", want: "collector.timeout"},
		"idle exceeds max":     {yaml: "auth:\n  session_idle_timeout: 13h\n", want: "session_idle_timeout"},
		"bad listen":           {env: []string{"FLUI_SERVER_LISTEN=8443"}, want: "server.listen"},
		"bad log level":        {yaml: "log:\n  level: trace\n", want: "log.level"},
		"weak password policy": {yaml: "auth:\n  password_min_length: 4\n", want: "password_min_length"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Load(writeFile(t, tc.yaml), true, tc.env)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestParseDuration(t *testing.T) {
	cases := map[string]time.Duration{"30s": 30 * time.Second, "15m": 15 * time.Minute, "2d": 2 * Day, "1.5d": 36 * time.Hour}
	for in, want := range cases {
		got, err := ParseDuration(in)
		if err != nil || got.D() != want {
			t.Errorf("ParseDuration(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "d", "-1d", "abc"} {
		if _, err := ParseDuration(in); err == nil {
			t.Errorf("ParseDuration(%q) should fail", in)
		}
	}
	if s := Duration(30 * Day).String(); s != "30d" {
		t.Errorf("String() = %q", s)
	}
}
