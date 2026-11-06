// Package pki manages flui's internal certificate authority and the self-signed server certificate.
package pki

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/ahmettsezis/flui/internal/secret"
)

const (
	CACertFile     = "ca.pem"
	caKeyFile      = "ca-key.enc"
	ServerCertFile = "server.pem"
	ServerKeyFile  = "server-key.pem"

	caValidity     = 10 * 365 * 24 * time.Hour
	serverValidity = 365 * 24 * time.Hour
	renewBefore    = 30 * 24 * time.Hour
)

var caKeyAAD = []byte("flui/pki/ca-key")

type CA struct {
	Cert    *x509.Certificate
	CertPEM []byte
	key     crypto.Signer
}

// Fingerprint returns the SHA-256 fingerprint of a DER certificate as "sha256:<hex>".
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// FileFingerprint returns the fingerprint of the first certificate in a PEM file.
func FileFingerprint(path string) (string, error) {
	c, err := readCert(path)
	if err != nil {
		return "", err
	}
	return Fingerprint(c.Raw), nil
}

// LoadOrCreateCA loads the CA from dir or creates a new one. The CA private key is stored encrypted with kr.
func LoadOrCreateCA(dir string, kr *secret.Keyring) (*CA, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	certPath, keyPath := filepath.Join(dir, CACertFile), filepath.Join(dir, caKeyFile)

	cert, err := readCert(certPath)
	if errors.Is(err, fs.ErrNotExist) {
		return createCA(certPath, keyPath, kr)
	}
	if err != nil {
		return nil, err
	}
	sealed, err := os.ReadFile(keyPath) //nolint:gosec // path is inside data_dir
	if err != nil {
		return nil, err
	}
	der, err := kr.Open(sealed, caKeyAAD)
	if err != nil {
		return nil, fmt.Errorf("pki: cannot decrypt CA key (wrong master key?): %w", err)
	}
	key, err := x509.ParsePKCS8PrivateKey(der)
	if err != nil {
		return nil, err
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, errors.New("pki: CA key is not a signer")
	}
	return &CA{Cert: cert, CertPEM: encodeCert(cert.Raw), key: signer}, nil
}

func createCA(certPath, keyPath string, kr *secret.Keyring) (*CA, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          randomSerial(),
		Subject:               pkix.Name{CommonName: "flui internal CA", Organization: []string{"flui"}},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(caValidity),
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLenZero:        true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	sealed, err := kr.Seal(keyDER, caKeyAAD)
	if err != nil {
		return nil, err
	}
	if err := writeFileAtomic(keyPath, sealed, 0o600); err != nil {
		return nil, err
	}
	certPEM := encodeCert(der)
	if err := writeFileAtomic(certPath, certPEM, 0o644); err != nil {
		return nil, err
	}
	return &CA{Cert: cert, CertPEM: certPEM, key: key}, nil
}

// EnsureServerCert makes sure dir contains a server certificate signed by ca that covers hosts and is not close to
// expiry. It issues a new one otherwise and reports whether it did.
func (ca *CA) EnsureServerCert(dir string, hosts []string, now time.Time) (certPath, keyPath string, issued bool, err error) {
	certPath, keyPath = filepath.Join(dir, ServerCertFile), filepath.Join(dir, ServerKeyFile)
	if cur, err := readCert(certPath); err == nil && ca.serverCertValid(cur, hosts, now) {
		if _, err := os.Stat(keyPath); err == nil {
			return certPath, keyPath, false, nil
		}
	}

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", false, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: randomSerial(),
		Subject:      pkix.Name{CommonName: "flui", Organization: []string{"flui"}},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(serverValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.Cert, key.Public(), ca.key)
	if err != nil {
		return "", "", false, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return "", "", false, err
	}
	if err := writeFileAtomic(keyPath, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		return "", "", false, err
	}
	chain := append(encodeCert(der), ca.CertPEM...)
	if err := writeFileAtomic(certPath, chain, 0o644); err != nil {
		return "", "", false, err
	}
	return certPath, keyPath, true, nil
}

func (ca *CA) serverCertValid(c *x509.Certificate, hosts []string, now time.Time) bool {
	if c.CheckSignatureFrom(ca.Cert) != nil || now.Add(renewBefore).After(c.NotAfter) || now.Before(c.NotBefore) {
		return false
	}
	for _, h := range hosts {
		if c.VerifyHostname(h) != nil {
			return false
		}
	}
	return true
}

// ServerHosts returns the names and addresses the self-signed server certificate should cover.
func ServerHosts(extra ...string) []string {
	hosts := []string{"localhost", "127.0.0.1", "::1"}
	if h, err := os.Hostname(); err == nil && h != "" {
		hosts = append(hosts, h)
	}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.IsGlobalUnicast() {
				hosts = append(hosts, ipn.IP.String())
			}
		}
	}
	for _, h := range extra {
		if h != "" && h != "0.0.0.0" && h != "::" {
			hosts = append(hosts, h)
		}
	}
	slices.Sort(hosts)
	return slices.Compact(hosts)
}

func readCert(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is inside data_dir or set by the operator
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("pki: %s does not contain a PEM certificate", path)
	}
	return x509.ParseCertificate(block.Bytes)
}

func encodeCert(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func randomSerial() *big.Int {
	n, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 127))
	if err != nil {
		panic(err)
	}
	return n
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
