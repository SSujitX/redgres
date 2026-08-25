package postgresadmin

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/secrets"
)

type python49Fixtures struct {
	SessionSecret    string `json:"session_secret"`
	DerivedFernetKey string `json:"derived_fernet_key"`
	ASCII            struct {
		Plaintext string `json:"plaintext"`
		Token     string `json:"token"`
	} `json:"ascii"`
	TamperedCiphertext string `json:"tampered_ciphertext"`
}

func loadPython49(t *testing.T) python49Fixtures {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "secrets", "testdata", "python49.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx python49Fixtures
	if err := json.Unmarshal(raw, &fx); err != nil {
		t.Fatal(err)
	}
	if fx.ASCII.Plaintext == "" || fx.ASCII.Token == "" || fx.SessionSecret == "" {
		t.Fatal("python49 fixture missing ASCII canary")
	}
	return fx
}

func revealCatalog(t *testing.T, fx python49Fixtures) *MemoryCatalog {
	t.Helper()
	return &MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "project_a_role")},
		Ciphertexts: map[string]string{
			"project_a_role": fx.ASCII.Token,
		},
	}
}

func revealService(t *testing.T, cat *MemoryCatalog, fx python49Fixtures) *Service {
	t.Helper()
	svc := NewService(cat, NewPolicy(config.Config{
		PostgresDatabase: "postgres",
		PostgresUser:     "redgres_console",
	}))
	svc.vaultKey = secrets.DeriveVaultKey(fx.SessionSecret)
	return svc
}

func TestServiceRevealDecryptsPython49ASCII(t *testing.T) {
	fx := loadPython49(t)
	cat := revealCatalog(t, fx)
	svc := revealService(t, cat, fx)
	got, err := svc.Reveal(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.Database != "project_a" || got.Owner != "project_a_role" {
		t.Fatalf("identity = %#v", got)
	}
	if got.Password != fx.ASCII.Plaintext {
		t.Fatalf("password mismatch")
	}
	if len(cat.EncryptedPasswordCalls) != 1 || cat.EncryptedPasswordCalls[0] != "project_a_role" {
		t.Fatalf("SELECT calls = %#v", cat.EncryptedPasswordCalls)
	}
}

func TestServiceRevealMissingRowIsNotFoundWithoutDecrypt(t *testing.T) {
	fx := loadPython49(t)
	cat := &MemoryCatalog{
		Rows:        []CatalogRow{projectRow("project_a", "project_a_role")},
		Ciphertexts: map[string]string{},
	}
	svc := revealService(t, cat, fx)
	_, err := svc.Reveal(context.Background(), "project_a")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(cat.EncryptedPasswordCalls) != 1 {
		t.Fatalf("must SELECT before 404: %#v", cat.EncryptedPasswordCalls)
	}
}

func TestServiceRevealVaultUnavailableIsUnavailable(t *testing.T) {
	fx := loadPython49(t)
	cat := &MemoryCatalog{
		Rows:          []CatalogRow{projectRow("project_a", "project_a_role")},
		CiphertextErr: ErrVaultUnavailable,
	}
	svc := revealService(t, cat, fx)
	_, err := svc.Reveal(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, ErrNotFound) {
		t.Fatal("vault unavailable must not be 404")
	}
}

func TestServiceRevealUnsetKeyIsUnavailable(t *testing.T) {
	fx := loadPython49(t)
	cat := revealCatalog(t, fx)
	svc := NewService(cat, NewPolicy(config.Config{}))
	_, err := svc.Reveal(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if len(cat.EncryptedPasswordCalls) != 1 {
		t.Fatalf("must SELECT before key check: %#v", cat.EncryptedPasswordCalls)
	}
}

func TestServiceRevealInvalidTokenIsUnavailable(t *testing.T) {
	fx := loadPython49(t)
	cat := &MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "project_a_role")},
		Ciphertexts: map[string]string{
			"project_a_role": fx.TamperedCiphertext,
		},
	}
	svc := revealService(t, cat, fx)
	_, err := svc.Reveal(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), fx.ASCII.Plaintext) || strings.Contains(err.Error(), fx.TamperedCiphertext) || strings.Contains(err.Error(), fx.SessionSecret)) {
		t.Fatalf("leaked canary: %v", err)
	}
}

func TestServiceRevealCanaryAbsentFromErrors(t *testing.T) {
	fx := loadPython49(t)
	canary := "postgresql://canary-secret@10.0.0.1/db"
	cat := &MemoryCatalog{
		Rows:          []CatalogRow{projectRow("project_a", "project_a_role")},
		CiphertextErr: errors.New(canary),
	}
	svc := revealService(t, cat, fx)
	_, err := svc.Reveal(context.Background(), "project_a")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), "canary-secret") || strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), fx.ASCII.Token) || strings.Contains(err.Error(), fx.ASCII.Plaintext)) {
		t.Fatalf("leaked canary: %v", err)
	}
}

func TestServiceRevealEmptyOwnerIsNotFoundWithoutSelect(t *testing.T) {
	fx := loadPython49(t)
	cat := &MemoryCatalog{
		Rows: []CatalogRow{projectRow("project_a", "")},
		Ciphertexts: map[string]string{
			"": fx.ASCII.Token,
		},
	}
	svc := revealService(t, cat, fx)
	_, err := svc.Reveal(context.Background(), "project_a")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
	if len(cat.EncryptedPasswordCalls) != 0 {
		t.Fatalf("empty owner must not SELECT: %#v", cat.EncryptedPasswordCalls)
	}
}

func TestServiceRevealProtectedNeverSelects(t *testing.T) {
	fx := loadPython49(t)
	cat := &MemoryCatalog{
		Rows: []CatalogRow{
			projectRow("project_a", "project_a_role"),
			{Name: "postgres", Owner: "postgres", AllowConn: true},
		},
		Ciphertexts: map[string]string{
			"postgres":       fx.ASCII.Token,
			"project_a_role": fx.ASCII.Token,
		},
	}
	svc := revealService(t, cat, fx)
	for _, name := range []string{"postgres", "template0", "template1", "database_console_vault", "missing_db"} {
		_, err := svc.Reveal(context.Background(), name)
		if !errors.Is(err, ErrNotFound) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if len(cat.EncryptedPasswordCalls) != 0 {
		t.Fatalf("protected must not SELECT: %#v", cat.EncryptedPasswordCalls)
	}
}

func TestServiceConnectionDoesNotSelectCiphertext(t *testing.T) {
	fx := loadPython49(t)
	cat := revealCatalog(t, fx)
	cat.SavedRoles = []string{"project_a_role"}
	svc := revealService(t, cat, fx)
	got, err := svc.Connection(context.Background(), "project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got.SavedCredential.Status != "present" {
		t.Fatalf("credential = %#v", got.SavedCredential)
	}
	if len(cat.EncryptedPasswordCalls) != 0 {
		t.Fatalf("GET connection must not SELECT ciphertext: %#v", cat.EncryptedPasswordCalls)
	}
}

func TestEncryptedPasswordSQLSelectsCiphertextOnly(t *testing.T) {
	blob := strings.ToLower(encryptedPasswordSQL)
	if blob != "select encrypted_password from public.project_credentials where role_name = $1" {
		t.Fatalf("sql = %s", encryptedPasswordSQL)
	}
	if strings.Contains(blob, "updated_at") {
		t.Fatal("must not select updated_at")
	}
	if strings.Contains(strings.ToLower(savedRoleNamesSQL), "encrypted_password") {
		t.Fatal("existence SQL must stay ciphertext-free")
	}
}

func TestPoolCatalogEncryptedPasswordNilPoolIsUnavailable(t *testing.T) {
	var c PoolCatalog
	_, err := c.EncryptedPassword(context.Background(), "project_a_role")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if errors.Is(err, ErrVaultUnavailable) {
		t.Fatal("reveal vault failure must be ErrUnavailable, not GET not_available")
	}
}
