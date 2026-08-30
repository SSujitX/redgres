package tlsops

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteCloudflareDNSCredentialsFormatAndMode(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "certbot-dns.ini")
	const token = "canary-cf-dns-token"
	if err := WriteCloudflareDNSCredentials(path, token); err != nil {
		t.Fatalf("WriteCloudflareDNSCredentials: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "dns_cloudflare_api_token = "+token+"\n" {
		t.Fatalf("ini = %q", raw)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestWriteCloudflareDNSCredentialsRejectsEmptyOrUnsafeToken(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "certbot-dns.ini")
	for _, token := range []string{"", "  ", "a\nb", "a=b", "has space"} {
		if err := WriteCloudflareDNSCredentials(path, token); err == nil {
			t.Fatalf("expected error for token %q", token)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("file created for rejected token %q", token)
		}
	}
}

func TestWriteTLSIssueRequestHostnamesOnly(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tls-issue.request")
	if err := WriteTLSIssueRequest(path, []string{"db.example.com", "rs.example.com"}); err != nil {
		t.Fatalf("WriteTLSIssueRequest: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(string(raw))
	if got != "db.example.com\nrs.example.com" {
		t.Fatalf("request = %q", raw)
	}
}

func TestReadIssueResultIssuedOrFailed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	issued := filepath.Join(dir, "issued")
	failed := filepath.Join(dir, "failed")
	if err := os.WriteFile(issued, []byte("issued\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(failed, []byte("failed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ReadIssueResult(issued); got != "issued" {
		t.Fatalf("issued = %q", got)
	}
	if got := ReadIssueResult(failed); got != "failed" {
		t.Fatalf("failed = %q", got)
	}
	if got := ReadIssueResult(filepath.Join(dir, "missing")); got != "" {
		t.Fatalf("missing = %q", got)
	}
	bound := filepath.Join(dir, "bound")
	if err := os.WriteFile(bound, []byte("issued\ndb.example.com\nrs.example.com\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !IssueResultCovers(bound, []string{"db.example.com", "rs.example.com"}) {
		t.Fatal("expected cover")
	}
	if IssueResultCovers(bound, []string{"db.other.com", "rs.example.com"}) {
		t.Fatal("must not overlay issued for a different hostname")
	}
}

func TestWriteTLSIssueRequestRejectsInvalidHostname(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "tls-issue.request")
	if err := WriteTLSIssueRequest(path, []string{"db.example.com", "../etc/passwd"}); err == nil {
		t.Fatal("expected invalid hostname error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("request file created for invalid hostname")
	}
}
