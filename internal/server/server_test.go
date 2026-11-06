package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahmettsezis/flui/internal/config"
	"github.com/ahmettsezis/flui/internal/pki"
)

func TestRunServesHTTPSAndShutsDown(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = filepath.Join(t.TempDir(), "data")
	cfg.MasterKeyFile = filepath.Join(cfg.DataDir, "master.key")
	cfg.Server.Listen = "127.0.0.1:0"

	ln, err := net.Listen("tcp", cfg.Server.Listen)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, slog.New(slog.DiscardHandler), ln) }()

	caPath := filepath.Join(TLSDir(cfg), pki.CACertFile)
	var caPEM []byte
	deadline := time.Now().Add(10 * time.Second)
	for {
		if caPEM, err = os.ReadFile(caPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("CA not created: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(caPEM)
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "localhost"}}}
	url := "https://" + ln.Addr().String()

	var resp *http.Response
	for {
		if resp, err = client.Get(url + "/readyz"); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.Header.Get("Strict-Transport-Security") == "" {
		t.Fatalf("readyz: status=%d body=%s", resp.StatusCode, b)
	}
	if resp.TLS.Version < tls.VersionTLS12 {
		t.Errorf("negotiated TLS version %x", resp.TLS.Version)
	}

	old := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{
		RootCAs: pool, ServerName: "localhost", MinVersion: tls.VersionTLS11, MaxVersion: tls.VersionTLS11, //nolint:gosec // testing that old TLS is rejected
	}}}
	if r, err := old.Get(url + "/healthz"); err == nil {
		r.Body.Close()
		t.Error("TLS 1.1 connection was accepted")
	}

	for _, f := range []string{cfg.MasterKeyFile, filepath.Join(TLSDir(cfg), pki.ServerKeyFile)} {
		if st, err := os.Stat(f); err != nil || st.Mode().Perm() != 0o600 {
			t.Errorf("%s: mode=%v err=%v", f, st.Mode().Perm(), err)
		}
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestPublicURL(t *testing.T) {
	cfg := config.Default()
	cfg.Server.PublicURL = "https://flui.example.com"
	if got := PublicURL(cfg); got != "https://flui.example.com" {
		t.Errorf("PublicURL = %q", got)
	}
	cfg.Server.PublicURL = ""
	cfg.Server.Listen = "10.0.0.5:9443"
	if got := PublicURL(cfg); got != "https://10.0.0.5:9443" {
		t.Errorf("PublicURL = %q", got)
	}
}
