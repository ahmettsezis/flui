// Package server wires flui-server's components together and runs the HTTPS server.
package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/ahmettsezis/flui/internal/api"
	"github.com/ahmettsezis/flui/internal/auth"
	"github.com/ahmettsezis/flui/internal/collector"
	"github.com/ahmettsezis/flui/internal/config"
	"github.com/ahmettsezis/flui/internal/nodes"
	"github.com/ahmettsezis/flui/internal/pki"
	"github.com/ahmettsezis/flui/internal/secret"
	"github.com/ahmettsezis/flui/internal/store"
	"github.com/ahmettsezis/flui/internal/version"
	"github.com/ahmettsezis/flui/internal/webui"
)

const certRenewCheck = 12 * time.Hour

func NewLogger(c config.Log, w io.Writer) *slog.Logger {
	var level slog.Level
	_ = level.UnmarshalText([]byte(c.Level))
	opts := &slog.HandlerOptions{Level: level}
	if c.Format == "text" {
		return slog.New(slog.NewTextHandler(w, opts))
	}
	return slog.New(slog.NewJSONHandler(w, opts))
}

func TLSDir(cfg config.Config) string { return filepath.Join(cfg.DataDir, "tls") }

// TLSConfig returns flui's server TLS settings: TLS 1.2+ with AEAD ciphers only.
func TLSConfig(getCert func(*tls.ClientHelloInfo) (*tls.Certificate, error)) *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS12,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256,
			tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
		},
		GetCertificate: getCert,
	}
}

// Run starts flui-server and blocks until ctx is cancelled. If ln is nil it listens on cfg.Server.Listen.
func Run(ctx context.Context, cfg config.Config, log *slog.Logger, ln net.Listener) error {
	log.Info("starting flui-server", "version", version.Version, "commit", version.Commit)

	if err := os.MkdirAll(cfg.DataDir, 0o700); err != nil {
		return err
	}
	if st, err := os.Stat(cfg.DataDir); err == nil && st.Mode().Perm()&0o077 != 0 {
		if err := os.Chmod(cfg.DataDir, 0o700); err != nil { //nolint:gosec // directory needs the execute bit
			log.Warn("data_dir is accessible by other users; restrict it with chmod 0700", "data_dir", cfg.DataDir, "err", err)
		}
	}

	kr, created, insecure, err := secret.LoadOrCreate(cfg.MasterKeyFile)
	if err != nil {
		return fmt.Errorf("master key: %w", err)
	}
	if created {
		log.Warn("created a new master key; back it up separately from the database", "path", cfg.MasterKeyFile)
	}
	if insecure {
		log.Warn("master key file is readable by group or others; restrict it with chmod 0600", "path", cfg.MasterKeyFile)
	}

	st, err := store.Open(ctx, cfg.DataDir)
	if err != nil {
		return err
	}
	defer st.Close()
	applied, err := st.Migrate(ctx)
	if err != nil {
		return err
	}
	if len(applied) > 0 {
		log.Info("applied database migrations", "versions", applied)
	}

	certs, err := setupTLS(ctx, cfg, kr, log)
	if err != nil {
		return err
	}

	authSvc, err := auth.NewService(st.DB, cfg.Auth)
	if err != nil {
		return err
	}
	pwFile := InitialPasswordFile(cfg)
	if err := bootstrapAdmin(ctx, authSvc, pwFile, log); err != nil {
		return err
	}
	repo := nodes.NewRepo(st.DB)
	coll := collector.New(st.DB, repo, cfg.Collector, log)
	go coll.Run(ctx)
	go runJobs(ctx, cfg, st.DB, authSvc, log)

	handler := api.New(api.Options{
		DB: st, SQL: st.DB, Logger: log, UI: webui.FS(), Auth: authSvc, Nodes: repo, Collector: coll, Config: cfg,
		OnPasswordChanged: func(*auth.User) {
			if err := os.Remove(pwFile); err == nil {
				log.Info("removed initial admin password file", "path", pwFile)
			}
		},
	})

	srv := &http.Server{
		Handler:           handler,
		TLSConfig:         TLSConfig(certs.GetCertificate),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelDebug),
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	if ln == nil {
		if ln, err = new(net.ListenConfig).Listen(ctx, "tcp", cfg.Server.Listen); err != nil {
			return err
		}
	}

	errc := make(chan error, 1)
	go func() { errc <- srv.ServeTLS(ln, "", "") }()
	log.Info("flui-server is ready", "listen", ln.Addr().String(), "url", PublicURL(cfg))

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// setupTLS returns the certificate source for the HTTPS listener. The internal CA signs the server
// certificate when no certificate is configured.
func setupTLS(ctx context.Context, cfg config.Config, kr *secret.Keyring, log *slog.Logger) (*pki.Reloader, error) {
	onErr := func(err error) { log.Error("reloading TLS certificate failed; keeping the previous one", "err", err) }
	ca, err := pki.LoadOrCreateCA(TLSDir(cfg), kr)
	if err != nil {
		return nil, err
	}
	log.Info("internal CA", "fingerprint", pki.Fingerprint(ca.Cert.Raw))

	if cfg.Server.TLS.CertFile != "" {
		r, err := pki.NewReloader(cfg.Server.TLS.CertFile, cfg.Server.TLS.KeyFile, onErr)
		if err != nil {
			return nil, fmt.Errorf("server.tls: %w", err)
		}
		log.Info("using configured TLS certificate", "cert_file", cfg.Server.TLS.CertFile)
		return r, nil
	}

	hosts := pki.ServerHosts(hostOf(cfg.Server.PublicURL), listenHost(cfg.Server.Listen))
	ensure := func() (string, string, error) {
		certPath, keyPath, issued, err := ca.EnsureServerCert(TLSDir(cfg), hosts, time.Now())
		if err == nil && issued {
			fp, _ := pki.FileFingerprint(certPath)
			log.Info("issued self-signed server certificate", "hosts", hosts, "fingerprint", fp)
		}
		return certPath, keyPath, err
	}
	certPath, keyPath, err := ensure()
	if err != nil {
		return nil, err
	}
	r, err := pki.NewReloader(certPath, keyPath, onErr)
	if err != nil {
		return nil, err
	}
	go func() {
		t := time.NewTicker(certRenewCheck)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if _, _, err := ensure(); err != nil {
					log.Error("renewing self-signed server certificate failed", "err", err)
				}
			}
		}
	}()
	return r, nil
}

// PublicURL returns server.public_url, or a URL derived from the hostname and listen port.
func PublicURL(cfg config.Config) string {
	if cfg.Server.PublicURL != "" {
		return cfg.Server.PublicURL
	}
	host, port, _ := net.SplitHostPort(cfg.Server.Listen)
	if host == "" || host == "0.0.0.0" || host == "::" {
		if h, err := os.Hostname(); err == nil {
			host = h
		}
	}
	return "https://" + net.JoinHostPort(host, port)
}

func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}

func listenHost(listen string) string {
	h, _, _ := net.SplitHostPort(listen)
	return h
}
