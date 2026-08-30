package tlsops

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/SSujitX/redgres/internal/securefile"
)

type IssueFailureCode string

const (
	IssueFailureRateLimited IssueFailureCode = "rate_limited"
	IssueFailureBusy        IssueFailureCode = "busy"
	IssueFailureDNS         IssueFailureCode = "dns"
	IssueFailureCredentials IssueFailureCode = "credentials"
	IssueFailureDependency  IssueFailureCode = "dependency"
)

type IssueFailure struct {
	Code       IssueFailureCode
	RetryAfter time.Time
	Hostnames  map[string]struct{}
}

func (f IssueFailure) Covers(hostnames []string) bool {
	if len(hostnames) == 0 || len(f.Hostnames) == 0 {
		return false
	}
	for _, host := range hostnames {
		if _, ok := f.Hostnames[strings.TrimSpace(strings.ToLower(host))]; !ok {
			return false
		}
	}
	return true
}

func IsIssueFailureCode(code string) bool {
	switch IssueFailureCode(code) {
	case IssueFailureRateLimited, IssueFailureBusy, IssueFailureDNS, IssueFailureCredentials, IssueFailureDependency:
		return true
	default:
		return false
	}
}

// WriteCloudflareDNSCredentials writes a certbot-dns-cloudflare ini (0600).
// The token is never logged; callers must not print the file.
func WriteCloudflareDNSCredentials(path, token string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("certbot dns credentials path is empty")
	}
	token = strings.TrimSpace(token)
	if token == "" || strings.ContainsAny(token, " \t\r\n=") {
		return errors.New("invalid cloudflare dns token")
	}
	return writeSecureFileAtomic(path, []byte("dns_cloudflare_api_token = "+token+"\n"))
}

// WriteTLSIssueRequest writes hostnames only (no credentials) for the root helper.
func WriteTLSIssueRequest(path string, hostnames []string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("tls issue request path is empty")
	}
	if len(hostnames) != 2 {
		return errors.New("tls issue requires exactly two hostnames")
	}
	var b strings.Builder
	for i, host := range hostnames {
		host = strings.TrimSpace(strings.ToLower(host))
		if !validIssueHostname(host) {
			return errors.New("invalid hostname for tls issue")
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(host)
	}
	b.WriteByte('\n')
	return writeSecureFileAtomic(path, []byte(b.String()))
}

// WriteTLSCredentialCleanupRequest asks the fixed root helper to remove its
// renewal-only credential snapshot. Deployed copies and the trusted Certbot
// lineage stay available for recovery and a later explicit uninstall.
func WriteTLSCredentialCleanupRequest(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("tls cleanup request path is empty")
	}
	return writeSecureFileAtomic(path, []byte("cleanup_domain_tls\n"))
}

// ReadIssueResult returns "issued", "failed", or "" if missing/unknown. Never a secret.
func ReadIssueResult(path string) string {
	status, _ := parseIssueResult(path)
	return status
}

// ReadIssueFailure returns only allow-listed helper state. Unrecognized lines,
// including raw dependency output, are ignored.
func ReadIssueFailure(path string) IssueFailure {
	failure := IssueFailure{Code: IssueFailureDependency, Hostnames: map[string]struct{}{}}
	path = strings.TrimSpace(path)
	if path == "" {
		return failure
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return failure
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	if len(lines) == 0 || strings.TrimSpace(strings.ToLower(lines[0])) != "failed" {
		return failure
	}
	for _, line := range lines[1:] {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(key)) {
		case "reason":
			code := strings.TrimSpace(strings.ToLower(value))
			if IsIssueFailureCode(code) {
				failure.Code = IssueFailureCode(code)
			}
		case "retry_after":
			parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value))
			if err == nil {
				failure.RetryAfter = parsed.UTC()
			}
		case "host":
			host := strings.TrimSpace(strings.ToLower(value))
			if validIssueHostname(host) {
				failure.Hostnames[host] = struct{}{}
			}
		}
	}
	if failure.Code != IssueFailureRateLimited {
		failure.RetryAfter = time.Time{}
	}
	return failure
}

// IssueResultCovers is true only when the helper issued certs for every hostname.
func IssueResultCovers(path string, hostnames []string) bool {
	status, covered := parseIssueResult(path)
	if status != "issued" || len(hostnames) == 0 {
		return false
	}
	for _, host := range hostnames {
		host = strings.TrimSpace(strings.ToLower(host))
		if _, ok := covered[host]; !ok {
			return false
		}
	}
	return true
}

// ReadIssueEndpointStatuses returns only the two stable application states
// published by the privileged helper. Certificate coverage alone is not proof
// that a service is presenting TLS.
func ReadIssueEndpointStatuses(path string) map[string]string {
	out := map[string]string{}
	raw, err := os.ReadFile(strings.TrimSpace(path))
	if err != nil {
		return out
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(strings.ToLower(line)), "=")
		if !ok || (key != "db_status" && key != "rs_status") {
			continue
		}
		if value == "issued" || value == "certificate_prepared" {
			out[strings.TrimSuffix(key, "_status")] = value
		}
	}
	return out
}

func parseIssueResult(path string) (string, map[string]struct{}) {
	covered := map[string]struct{}{}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", covered
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", covered
	}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	status := ""
	for _, line := range lines {
		line = strings.TrimSpace(strings.ToLower(line))
		if line == "" {
			continue
		}
		if status == "" {
			if line == "issued" || line == "failed" {
				status = line
			}
			continue
		}
		if validIssueHostname(line) {
			covered[line] = struct{}{}
		}
	}
	return status, covered
}

func writeSecureFileAtomic(path string, body []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".tls-issue-request-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err := f.Chmod(0o600); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(body); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeSecureFile(path string, body []byte) error {
	f, err := securefile.OpenRegular(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Write(body); err != nil {
		return err
	}
	return f.Sync()
}

func validIssueHostname(s string) bool {
	if len(s) == 0 || len(s) > 253 || !strings.Contains(s, ".") {
		return false
	}
	if strings.ContainsAny(s, " /:@?%#\\*") {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if label == "" || len(label) > 63 {
			return false
		}
		if label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
				continue
			}
			return false
		}
	}
	return true
}
