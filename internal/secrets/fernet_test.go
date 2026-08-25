package secrets

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type python49Fixtures struct {
	CryptographyVersion string `json:"cryptography_version"`
	KDFPrefix           string `json:"kdf_prefix"`
	SessionSecret       string `json:"session_secret"`
	DerivedFernetKey    string `json:"derived_fernet_key"`
	WrongSessionSecret  string `json:"wrong_session_secret"`
	WrongFernetKey      string `json:"wrong_fernet_key"`
	ASCII               struct {
		Plaintext string `json:"plaintext"`
		Token     string `json:"token"`
	} `json:"ascii"`
	Unicode struct {
		Plaintext string `json:"plaintext"`
		Token     string `json:"token"`
	} `json:"unicode"`
	OldTimestamp struct {
		Plaintext     string `json:"plaintext"`
		Token         string `json:"token"`
		TimestampUnix int64  `json:"timestamp_unix"`
	} `json:"old_timestamp"`
	TamperedCiphertext string `json:"tampered_ciphertext"`
	WrongVersion       string `json:"wrong_version"`
	Truncated          string `json:"truncated"`
	InvalidBase64      string `json:"invalid_base64"`
}

func loadPython49(t *testing.T) python49Fixtures {
	t.Helper()
	path := filepath.Join("testdata", "python49.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fx python49Fixtures
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if fx.CryptographyVersion != "49.0.0" {
		t.Fatalf("fixture cryptography_version %q, want 49.0.0", fx.CryptographyVersion)
	}
	if fx.KDFPrefix != "database-console-vault-v1:" {
		t.Fatalf("fixture kdf prefix %q", fx.KDFPrefix)
	}
	return fx
}

func TestDeriveVaultKeyMatchesPython49(t *testing.T) {
	fx := loadPython49(t)
	got := DeriveVaultKey(fx.SessionSecret)
	if got != fx.DerivedFernetKey {
		t.Fatalf("KDF mismatch: got %q want %q", got, fx.DerivedFernetKey)
	}
	if DeriveVaultKey(fx.WrongSessionSecret) != fx.WrongFernetKey {
		t.Fatal("wrong-secret KDF does not match Python fixture")
	}
}

func TestDecryptASCIIPassword(t *testing.T) {
	fx := loadPython49(t)
	got, err := Decrypt(DeriveVaultKey(fx.SessionSecret), fx.ASCII.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(fx.ASCII.Plaintext)) {
		t.Fatalf("plaintext %q, want %q", got, fx.ASCII.Plaintext)
	}
}

func TestDecryptUnicodePassword(t *testing.T) {
	fx := loadPython49(t)
	got, err := Decrypt(DeriveVaultKey(fx.SessionSecret), fx.Unicode.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(fx.Unicode.Plaintext)) {
		t.Fatalf("plaintext %q, want %q", got, fx.Unicode.Plaintext)
	}
}

func TestDecryptOldTimestampSucceedsWithoutTTL(t *testing.T) {
	fx := loadPython49(t)
	if fx.OldTimestamp.TimestampUnix != 1262304000 {
		t.Fatalf("expected 2010-01-01 fixture timestamp, got %d", fx.OldTimestamp.TimestampUnix)
	}
	got, err := Decrypt(DeriveVaultKey(fx.SessionSecret), fx.OldTimestamp.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, []byte(fx.OldTimestamp.Plaintext)) {
		t.Fatalf("plaintext %q, want %q", got, fx.OldTimestamp.Plaintext)
	}
}

func TestDecryptWrongKeyFailsClosed(t *testing.T) {
	fx := loadPython49(t)
	_, err := Decrypt(fx.WrongFernetKey, fx.ASCII.Token)
	assertInvalidToken(t, err, fx, fx.ASCII.Token, fx.ASCII.Plaintext)
}

func TestDecryptTamperedCiphertextFailsClosed(t *testing.T) {
	fx := loadPython49(t)
	_, err := Decrypt(fx.DerivedFernetKey, fx.TamperedCiphertext)
	assertInvalidToken(t, err, fx, fx.TamperedCiphertext, fx.ASCII.Plaintext)
}

func TestDecryptTruncatedTokenFailsClosed(t *testing.T) {
	fx := loadPython49(t)
	_, err := Decrypt(fx.DerivedFernetKey, fx.Truncated)
	assertInvalidToken(t, err, fx, fx.Truncated, fx.ASCII.Plaintext)
}

func TestDecryptInvalidBase64FailsClosed(t *testing.T) {
	fx := loadPython49(t)
	_, err := Decrypt(fx.DerivedFernetKey, fx.InvalidBase64)
	assertInvalidToken(t, err, fx, fx.InvalidBase64, fx.ASCII.Plaintext)
}

func TestDecryptWrongVersionFailsClosed(t *testing.T) {
	fx := loadPython49(t)
	_, err := Decrypt(fx.DerivedFernetKey, fx.WrongVersion)
	assertInvalidToken(t, err, fx, fx.WrongVersion, fx.ASCII.Plaintext)
}

func assertInvalidToken(t *testing.T, err error, fx python49Fixtures, token, plaintext string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected ErrInvalidToken, got nil")
	}
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("got %v, want ErrInvalidToken", err)
	}
	canaries := []string{
		fx.SessionSecret,
		fx.WrongSessionSecret,
		fx.DerivedFernetKey,
		fx.WrongFernetKey,
		token,
		plaintext,
		fx.Unicode.Plaintext,
	}
	msg := err.Error()
	for _, canary := range canaries {
		if canary != "" && strings.Contains(msg, canary) {
			t.Fatalf("error leaked canary %q: %q", canary, msg)
		}
	}
}
