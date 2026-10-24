package admission

import (
	"crypto/tls"
	"fmt"
	"os"
	"sync"
	"time"
)

// CertReloader serves the TLS certificate from disk and picks up rotated
// certificates without a restart. cert-manager rewrites the mounted Secret,
// and the reloader notices the new modification time on the next handshake.
type CertReloader struct {
	certFile, keyFile string

	mu      sync.RWMutex
	cert    *tls.Certificate
	modTime time.Time
}

// NewCertReloader loads the certificate pair once and fails fast if it is unusable.
func NewCertReloader(certFile, keyFile string) (*CertReloader, error) {
	r := &CertReloader{certFile: certFile, keyFile: keyFile}
	if err := r.reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// GetCertificate implements tls.Config.GetCertificate. A failed reload keeps
// the last good certificate in service instead of breaking handshakes.
func (r *CertReloader) GetCertificate(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	_ = r.maybeReload()
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cert, nil
}

// TLSConfig returns a server TLS configuration that uses the reloader.
func (r *CertReloader) TLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:     tls.VersionTLS12,
		GetCertificate: r.GetCertificate,
	}
}

func (r *CertReloader) maybeReload() error {
	info, err := os.Stat(r.certFile)
	if err != nil {
		return err
	}
	r.mu.RLock()
	unchanged := info.ModTime().Equal(r.modTime)
	r.mu.RUnlock()
	if unchanged {
		return nil
	}
	return r.reload()
}

func (r *CertReloader) reload() error {
	info, err := os.Stat(r.certFile)
	if err != nil {
		return fmt.Errorf("stat certificate: %w", err)
	}
	pair, err := tls.LoadX509KeyPair(r.certFile, r.keyFile)
	if err != nil {
		return fmt.Errorf("loading key pair: %w", err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cert = &pair
	r.modTime = info.ModTime()
	return nil
}
