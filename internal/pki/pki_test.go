package pki

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ahmettsezis/flui/internal/secret"
)

func keyring(t *testing.T) *secret.Keyring {
	t.Helper()
	kr, err := secret.New(bytes.Repeat([]byte{7}, secret.KeySize))
	if err != nil {
		t.Fatal(err)
	}
	return kr
}

func TestCAIsPersistedAndKeyEncrypted(t *testing.T) {
	dir := t.TempDir()
	ca1, err := LoadOrCreateCA(dir, keyring(t))
	if err != nil {
		t.Fatal(err)
	}
	if !ca1.Cert.IsCA {
		t.Fatal("not a CA")
	}
	raw, err := os.ReadFile(filepath.Join(dir, caKeyFile))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("PRIVATE KEY")) {
		t.Fatal("CA key stored in plaintext")
	}

	ca2, err := LoadOrCreateCA(dir, keyring(t))
	if err != nil {
		t.Fatal(err)
	}
	if !ca2.Cert.Equal(ca1.Cert) {
		t.Fatal("CA was recreated")
	}

	other, _ := secret.New(bytes.Repeat([]byte{8}, secret.KeySize))
	if _, err := LoadOrCreateCA(dir, other); err == nil || !strings.Contains(err.Error(), "wrong master key") {
		t.Fatalf("wrong key: err = %v", err)
	}
}

func TestServerCertLifecycle(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir, keyring(t))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	hosts := []string{"localhost", "127.0.0.1", "flui.example.com"}

	certPath, keyPath, issued, err := ca.EnsureServerCert(dir, hosts, now)
	if err != nil || !issued {
		t.Fatalf("issue: issued=%v err=%v", issued, err)
	}
	pair, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(ca.Cert)
	for _, h := range hosts {
		if _, err := leaf.Verify(x509.VerifyOptions{DNSName: h, Roots: pool}); err != nil {
			t.Errorf("verify %s: %v", h, err)
		}
	}
	if st, _ := os.Stat(keyPath); st.Mode().Perm() != 0o600 {
		t.Errorf("server key mode = %v", st.Mode().Perm())
	}

	if _, _, issued, _ := ca.EnsureServerCert(dir, hosts, now); issued {
		t.Error("valid certificate was reissued")
	}
	if _, _, issued, _ := ca.EnsureServerCert(dir, append(hosts, "10.1.2.3"), now); !issued {
		t.Error("new host did not trigger reissue")
	}
	if _, _, issued, _ := ca.EnsureServerCert(dir, hosts, now.Add(serverValidity-renewBefore+time.Hour)); !issued {
		t.Error("expiring certificate was not renewed")
	}
}

func TestFingerprint(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir, keyring(t))
	if err != nil {
		t.Fatal(err)
	}
	fp, err := FileFingerprint(filepath.Join(dir, CACertFile))
	if err != nil {
		t.Fatal(err)
	}
	if fp != Fingerprint(ca.Cert.Raw) || !strings.HasPrefix(fp, "sha256:") || len(fp) != len("sha256:")+64 {
		t.Fatalf("fingerprint = %q", fp)
	}
}

func TestReloaderPicksUpNewCertificate(t *testing.T) {
	dir := t.TempDir()
	ca, err := LoadOrCreateCA(dir, keyring(t))
	if err != nil {
		t.Fatal(err)
	}
	certPath, keyPath, _, err := ca.EnsureServerCert(dir, []string{"a.example"}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r, err := NewReloader(certPath, keyPath, func(err error) { t.Errorf("reload error: %v", err) })
	if err != nil {
		t.Fatal(err)
	}
	r.interval = 0
	first, _ := r.GetCertificate(nil)

	if _, _, _, err := ca.EnsureServerCert(dir, []string{"b.example"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(time.Minute)
	for _, f := range []string{certPath, keyPath} {
		if err := os.Chtimes(f, future, future); err != nil {
			t.Fatal(err)
		}
	}
	second, _ := r.GetCertificate(nil)
	if bytes.Equal(first.Certificate[0], second.Certificate[0]) {
		t.Fatal("certificate was not reloaded")
	}
}

func TestServerHosts(t *testing.T) {
	hosts := ServerHosts("flui.example.com", "0.0.0.0", "")
	for _, want := range []string{"localhost", "127.0.0.1", "flui.example.com"} {
		if !slices.Contains(hosts, want) {
			t.Errorf("missing %s in %v", want, hosts)
		}
	}
	if slices.Contains(hosts, "0.0.0.0") || slices.Contains(hosts, "") {
		t.Errorf("unexpected wildcard in %v", hosts)
	}
}
