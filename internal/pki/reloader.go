package pki

import (
	"crypto/tls"
	"os"
	"sync"
	"time"
)

// Reloader serves a TLS certificate from disk and picks up changes (e.g. after certbot renewal) without a restart.
type Reloader struct {
	certFile, keyFile string
	interval          time.Duration
	onError           func(error)

	mu        sync.Mutex
	cert      *tls.Certificate
	modTime   time.Time
	lastCheck time.Time
}

func NewReloader(certFile, keyFile string, onError func(error)) (*Reloader, error) {
	r := &Reloader{certFile: certFile, keyFile: keyFile, interval: 10 * time.Second, onError: onError}
	if err := r.load(); err != nil {
		return nil, err
	}
	return r, nil
}

func (r *Reloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if time.Since(r.lastCheck) >= r.interval {
		r.lastCheck = time.Now()
		if mt := r.latestModTime(); !mt.Equal(r.modTime) {
			if err := r.load(); err != nil && r.onError != nil {
				r.onError(err)
			}
		}
	}
	return r.cert, nil
}

func (r *Reloader) load() error {
	mt := r.latestModTime()
	cert, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return err
	}
	r.cert, r.modTime = &cert, mt
	return nil
}

func (r *Reloader) latestModTime() time.Time {
	var latest time.Time
	for _, f := range []string{r.certFile, r.keyFile} {
		if st, err := os.Stat(f); err == nil && st.ModTime().After(latest) {
			latest = st.ModTime()
		}
	}
	return latest
}
