package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPersistBootstrapClosedRewritesEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redgres.env")
	original := "REDGRES_ENVIRONMENT=production\n" +
		"REDGRES_ADDRESS=127.0.0.1:8790\n" +
		"REDGRES_BOOTSTRAP_ADDRESS=0.0.0.0:8989\n" +
		"REDGRES_BASE_URL=http://203.0.113.10:8989\n" +
		"REDGRES_SQLITE_PATH=/var/lib/redgres/redgres.db\n" +
		"REDGRES_COOKIE_SECURE=false\n"
	if err := os.WriteFile(path, []byte(original), 0o640); err != nil {
		t.Fatal(err)
	}
	rewritten, err := PersistBootstrapClosed(path, "https://console.example.com")
	if err != nil || !rewritten {
		t.Fatalf("PersistBootstrapClosed: rewritten=%v err=%v", rewritten, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Contains(text, "0.0.0.0:8989") {
		t.Fatalf("bootstrap address still present: %s", text)
	}
	if !strings.Contains(text, "REDGRES_BOOTSTRAP_ADDRESS=\n") {
		t.Fatalf("expected empty bootstrap address: %s", text)
	}
	if !strings.Contains(text, "REDGRES_COOKIE_SECURE=true\n") {
		t.Fatalf("expected CookieSecure true: %s", text)
	}
	if !strings.Contains(text, "REDGRES_BASE_URL=https://console.example.com\n") {
		t.Fatalf("expected https base URL: %s", text)
	}
	if !strings.Contains(text, "REDGRES_SQLITE_PATH=/var/lib/redgres/redgres.db\n") {
		t.Fatalf("sqlite path must be preserved: %s", text)
	}
	if strings.Contains(text, "http://203.0.113.10:8989") {
		t.Fatalf("http bootstrap origin remained: %s", text)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && st.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %o, want 640", st.Mode().Perm())
	}
}

func TestPersistBootstrapClosedMissingFileIsNoop(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.env")
	rewritten, err := PersistBootstrapClosed(path, "https://console.example.com")
	if err != nil || rewritten {
		t.Fatalf("missing file should be no-op: rewritten=%v err=%v", rewritten, err)
	}
}

func TestPersistBootstrapClosedRejectsHTTPOrigin(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.env")
	if err := os.WriteFile(path, []byte("REDGRES_COOKIE_SECURE=false\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := PersistBootstrapClosed(path, "http://console.example.com"); err == nil {
		t.Fatal("expected http origin to fail")
	}
	got, _ := os.ReadFile(path)
	if !strings.Contains(string(got), "REDGRES_COOKIE_SECURE=false") {
		t.Fatal("failed persist must not rewrite the file")
	}
}

func TestPersistBootstrapClosedInPlaceWhenDirNotWritable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "redgres.env")
	if err := os.WriteFile(path, []byte("REDGRES_BOOTSTRAP_ADDRESS=0.0.0.0:8989\nREDGRES_COOKIE_SECURE=false\nREDGRES_BASE_URL=http://203.0.113.10:8989\n"), 0o660); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o550); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	tmp := path + ".tmp"
	if f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600); err == nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		t.Skip("platform allows creating a sibling in a 0550 directory")
	}
	rewritten, err := PersistBootstrapClosed(path, "https://console.example.com")
	if err != nil || !rewritten {
		t.Fatalf("in-place persist: rewritten=%v err=%v", rewritten, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "REDGRES_BASE_URL=https://console.example.com\n") {
		t.Fatalf("in-place rewrite failed: %s", got)
	}
}

func TestPersistBootstrapClosedUnwritableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "redgres.env")
	if err := os.WriteFile(path, []byte("REDGRES_COOKIE_SECURE=false\n"), 0o400); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0)
	if err == nil {
		_ = f.Close()
		t.Skip("platform allows write on 0400 file")
	}
	rewritten, err := PersistBootstrapClosed(path, "https://console.example.com")
	if err == nil || rewritten {
		t.Fatalf("expected write failure: rewritten=%v err=%v", rewritten, err)
	}
}
