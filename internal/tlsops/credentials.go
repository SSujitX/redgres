package tlsops

import (
	"errors"
	"os"
	"strings"

	"github.com/SSujitX/redgres/internal/securefile"
)

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
	return writeSecureFile(path, []byte("dns_cloudflare_api_token = "+token+"\n"))
}

// WriteTLSIssueRequest writes hostnames only (no credentials) for the root helper.
func WriteTLSIssueRequest(path string, hostnames []string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("tls issue request path is empty")
	}
	if len(hostnames) == 0 {
		return errors.New("no hostnames for tls issue")
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

// ReadIssueResult returns "issued", "failed", or "" if missing/unknown. Never a secret.
func ReadIssueResult(path string) string {
	status, _ := parseIssueResult(path)
	return status
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
	tmp := path + ".tmp"
	if err := writeSecureFile(tmp, body); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmp, path); err2 != nil {
			_ = os.Remove(tmp)
			return err2
		}
	}
	return nil
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
