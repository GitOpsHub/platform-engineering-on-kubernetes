package admission

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// writeSelfSigned writes a self-signed certificate with the given serial.
func writeSelfSigned(t *testing.T, certPath, keyPath string, serial int64) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: "policy-engine"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		DNSNames:     []string{"localhost"},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestCertReloaderPicksUpRotation(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	writeSelfSigned(t, certPath, keyPath, 1)

	r, err := NewCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	first, err := r.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if got := leafSerial(t, first); got.Int64() != 1 {
		t.Fatalf("initial serial = %v", got)
	}

	// Rotate: new material, and a later modification time so the reloader notices.
	writeSelfSigned(t, certPath, keyPath, 2)
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(certPath, later, later); err != nil {
		t.Fatal(err)
	}
	rotated, err := r.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatal(err)
	}
	if got := leafSerial(t, rotated); got.Int64() != 2 {
		t.Fatalf("rotated serial = %v, want 2", got)
	}
}

func TestCertReloaderRejectsMissingPair(t *testing.T) {
	if _, err := NewCertReloader(filepath.Join(t.TempDir(), "nope.crt"), "nope.key"); err == nil {
		t.Fatal("missing certificate must fail at start-up")
	}
}

func TestTLSConfigMinimumVersion(t *testing.T) {
	dir := t.TempDir()
	certPath, keyPath := filepath.Join(dir, "tls.crt"), filepath.Join(dir, "tls.key")
	writeSelfSigned(t, certPath, keyPath, 3)
	r, err := NewCertReloader(certPath, keyPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg := r.TLSConfig()
	if cfg.MinVersion != tls.VersionTLS12 || cfg.GetCertificate == nil {
		t.Fatalf("unexpected TLS config: min=%x getCert=%v", cfg.MinVersion, cfg.GetCertificate != nil)
	}
}

func leafSerial(t *testing.T, c *tls.Certificate) *big.Int {
	t.Helper()
	leaf, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return leaf.SerialNumber
}
