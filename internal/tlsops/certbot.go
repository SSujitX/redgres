package tlsops

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CertLiveDir is the certbot live certificate root (override in tests).
var CertLiveDir = "/etc/letsencrypt/live"

// Issuer runs certbot DNS-01 issuance with bounded arguments.
type Issuer struct {
	Bin                string
	DNSCredentialsFile string
}

// Issue runs certbot certonly for the given hostnames.
func (i Issuer) Issue(ctx context.Context, hostnames []string) error {
	if len(hostnames) == 0 {
		return errors.New("no hostnames for tls issue")
	}
	bin := strings.TrimSpace(i.Bin)
	if bin == "" {
		bin = "certbot"
	}
	creds := strings.TrimSpace(i.DNSCredentialsFile)
	if creds == "" {
		return errors.New("certbot dns credentials file is not configured")
	}
	args := []string{
		"certonly",
		"--non-interactive",
		"--agree-tos",
		"--register-unsafely-without-email",
		"--dns-cloudflare",
		"--dns-cloudflare-credentials", creds,
		"--cert-name", strings.TrimSpace(strings.ToLower(hostnames[0])),
		"--keep-until-expiring",
	}
	for _, h := range hostnames {
		h = strings.TrimSpace(strings.ToLower(h))
		if h == "" {
			return errors.New("invalid hostname for tls issue")
		}
		args = append(args, "-d", h)
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "HOME=/tmp"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("certbot failed: %w", err)
	}
	_ = out
	return nil
}

// VerifyCertFiles checks that hostnames are covered by a readable, unexpired cert chain.
// certbot stores multi-domain certificates under the first -d hostname only.
func VerifyCertFiles(hostnames []string, now time.Time) error {
	if len(hostnames) == 0 {
		return errors.New("no hostnames")
	}
	primary := strings.TrimSpace(strings.ToLower(hostnames[0]))
	if primary == "" {
		return errors.New("invalid hostname")
	}
	path := filepath.Join(CertLiveDir, primary, "fullchain.pem")
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("cert missing for %s", primary)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return fmt.Errorf("cert unreadable for %s", primary)
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("cert parse failed for %s", primary)
	}
	if now.After(cert.NotAfter) {
		return fmt.Errorf("cert expired for %s", primary)
	}
	covered := map[string]struct{}{strings.ToLower(cert.Subject.CommonName): {}}
	for _, name := range cert.DNSNames {
		covered[strings.ToLower(name)] = struct{}{}
	}
	for _, h := range hostnames {
		h = strings.TrimSpace(strings.ToLower(h))
		if h == "" {
			return errors.New("invalid hostname")
		}
		if _, ok := covered[h]; !ok {
			return fmt.Errorf("cert for %s does not cover %s", primary, h)
		}
	}
	return nil
}
