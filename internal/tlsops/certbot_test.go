package tlsops

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestVerifyCertFilesAcceptsSANCertUnderPrimaryHostname(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	old := CertLiveDir
	CertLiveDir = dir
	t.Cleanup(func() { CertLiveDir = old })

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	template := &x509.Certificate{
		Subject:               pkix.Name{CommonName: "db.example.com"},
		DNSNames:              []string{"db.example.com", "redis.example.com"},
		SerialNumber:          big.NewInt(1),
		NotAfter:              time.Now().UTC().Add(24 * time.Hour),
		NotBefore:             time.Now().UTC().Add(-time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	live := filepath.Join(dir, "db.example.com")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "fullchain.pem"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := VerifyCertFiles([]string{"db.example.com", "redis.example.com"}, time.Now().UTC()); err != nil {
		t.Fatalf("VerifyCertFiles: %v", err)
	}
}
