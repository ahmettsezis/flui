// Package config loads flui-server configuration from defaults, a YAML file and FLUI_* environment variables.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	DefaultPath  = "/etc/flui/flui.yaml"
	envPrefix    = "FLUI"
	maxRetention = 365 * Day
)

type Config struct {
	Server        Server    `yaml:"server"`
	DataDir       string    `yaml:"data_dir"`
	MasterKeyFile string    `yaml:"master_key_file"`
	Log           Log       `yaml:"log"`
	Auth          Auth      `yaml:"auth"`
	Collector     Collector `yaml:"collector"`
	Retention     Duration  `yaml:"retention"`
}

type Server struct {
	Listen         string   `yaml:"listen"`
	PublicURL      string   `yaml:"public_url"`
	TLS            TLS      `yaml:"tls"`
	TrustedProxies []string `yaml:"trusted_proxies"`
}

type TLS struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type Log struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type Auth struct {
	SessionIdleTimeout Duration `yaml:"session_idle_timeout"`
	SessionMaxLifetime Duration `yaml:"session_max_lifetime"`
	PasswordMinLength  int      `yaml:"password_min_length"`
	LockoutThreshold   int      `yaml:"lockout_threshold"`
	LockoutDuration    Duration `yaml:"lockout_duration"`
}

type Collector struct {
	Interval   Duration `yaml:"interval"`
	Timeout    Duration `yaml:"timeout"`
	StaleAfter Duration `yaml:"stale_after"`
}

func Default() Config {
	return Config{
		Server:  Server{Listen: ":8443"},
		DataDir: "/var/lib/flui",
		Log:     Log{Level: "info", Format: "json"},
		Auth: Auth{
			SessionIdleTimeout: Duration(30 * time.Minute),
			SessionMaxLifetime: Duration(12 * time.Hour),
			PasswordMinLength:  12,
			LockoutThreshold:   5,
			LockoutDuration:    Duration(15 * time.Minute),
		},
		Collector: Collector{
			Interval:   Duration(10 * time.Second),
			Timeout:    Duration(5 * time.Second),
			StaleAfter: Duration(30 * time.Second),
		},
		Retention: Duration(2 * Day),
	}
}

// Load reads the config file at path (if it exists), applies environment overrides and validates the result.
// A missing file is an error only when required is true.
func Load(path string, required bool, environ []string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path) //nolint:gosec // path comes from the operator
	switch {
	case err == nil:
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&cfg); err != nil && !errors.Is(err, io.EOF) {
			return cfg, fmt.Errorf("%s: %w", path, err)
		}
	case errors.Is(err, fs.ErrNotExist) && !required:
	default:
		return cfg, err
	}
	if err := applyEnv(&cfg, envPrefix, environ); err != nil {
		return cfg, err
	}
	if cfg.MasterKeyFile == "" {
		cfg.MasterKeyFile = filepath.Join(cfg.DataDir, "master.key")
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	var errs []error
	add := func(format string, a ...any) { errs = append(errs, fmt.Errorf(format, a...)) }

	if _, _, err := net.SplitHostPort(c.Server.Listen); err != nil {
		add("server.listen: %v", err)
	}
	if c.Server.PublicURL != "" {
		u, err := url.Parse(c.Server.PublicURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			add("server.public_url: must be an https:// URL")
		}
	}
	if (c.Server.TLS.CertFile == "") != (c.Server.TLS.KeyFile == "") {
		add("server.tls: cert_file and key_file must be set together")
	}
	for _, p := range c.Server.TrustedProxies {
		if _, _, err := net.ParseCIDR(p); err != nil {
			add("server.trusted_proxies: %q is not a CIDR", p)
		}
	}
	if c.DataDir == "" {
		add("data_dir: must not be empty")
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		add("log.level: must be debug, info, warn or error")
	}
	switch c.Log.Format {
	case "json", "text":
	default:
		add("log.format: must be json or text")
	}

	positive := map[string]Duration{
		"auth.session_idle_timeout": c.Auth.SessionIdleTimeout,
		"auth.session_max_lifetime": c.Auth.SessionMaxLifetime,
		"auth.lockout_duration":     c.Auth.LockoutDuration,
		"collector.interval":        c.Collector.Interval,
		"collector.timeout":         c.Collector.Timeout,
		"collector.stale_after":     c.Collector.StaleAfter,
	}
	for k, v := range positive {
		if v <= 0 {
			add("%s: must be greater than 0", k)
		}
	}
	if c.Auth.SessionIdleTimeout > c.Auth.SessionMaxLifetime {
		add("auth.session_idle_timeout: must not exceed auth.session_max_lifetime")
	}
	if c.Auth.PasswordMinLength < 8 {
		add("auth.password_min_length: must be at least 8")
	}
	if c.Auth.LockoutThreshold < 1 {
		add("auth.lockout_threshold: must be at least 1")
	}
	if c.Collector.Timeout >= c.Collector.Interval {
		add("collector.timeout: must be shorter than collector.interval")
	}
	if c.Collector.StaleAfter < c.Collector.Interval {
		add("collector.stale_after: must be at least collector.interval")
	}
	if c.Retention.D() < time.Hour || c.Retention.D() > maxRetention {
		add("retention: must be between 1h and %s", Duration(maxRetention))
	}
	return errors.Join(errs...)
}
