package postgresadmin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/SSujitX/redgres/internal/operations"
)

func TestDuplicateProbeUsesSavedRoleNamesWithoutDecrypt(t *testing.T) {
	cat := &MemoryCatalog{
		Rows: []CatalogRow{
			eligibleDuplicateRow("project_a", "app_project_a"),
			eligibleDuplicateRow("project_a_copy", "app_project_a_copy"),
		},
		CreatedRoles: []string{"app_project_a_copy"},
		SavedRoles:   []string{"app_project_a_copy"},
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	probe := NewDuplicateProbe(svc)
	outcome, err := probe.DuplicateState(context.Background(), operations.Operation{
		Target: "project_a_copy",
		Result: &operations.DuplicateResult{
			Database: "project_a_copy",
			Owner:    "app_project_a_copy",
			Source:   "project_a",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.CloneExists || !outcome.RoleExists || !outcome.VaultRowExists || outcome.Indeterminate {
		t.Fatalf("outcome = %#v", outcome)
	}
	if len(cat.EncryptedPasswordCalls) != 0 {
		t.Fatalf("probe decrypted: %#v", cat.EncryptedPasswordCalls)
	}
}

func TestDuplicateProbeNothingCreated(t *testing.T) {
	cat := &MemoryCatalog{Rows: []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")}}
	svc := duplicateService(t, cat, createVaultKey(t))
	outcome, err := NewDuplicateProbe(svc).DuplicateState(context.Background(), operations.Operation{
		Result: &operations.DuplicateResult{Database: "project_a_copy", Owner: "app_project_a_copy", Source: "project_a"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if outcome.CloneExists || outcome.RoleExists || outcome.VaultRowExists || outcome.Indeterminate {
		t.Fatalf("outcome = %#v", outcome)
	}
}

func TestDuplicateProbeVaultErrorIsUnavailable(t *testing.T) {
	cat := &MemoryCatalog{
		Rows:     []CatalogRow{eligibleDuplicateRow("project_a", "app_project_a")},
		VaultErr: errors.New("postgresql://canary-token:secret@10.0.0.1/db"),
	}
	svc := duplicateService(t, cat, createVaultKey(t))
	_, err := NewDuplicateProbe(svc).DuplicateState(context.Background(), operations.Operation{
		Result: &operations.DuplicateResult{Database: "project_a_copy", Owner: "app_project_a_copy", Source: "project_a"},
	})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
	if err != nil && strings.Contains(err.Error(), "canary-token") {
		t.Fatalf("leaked canary: %v", err)
	}
	if len(cat.EncryptedPasswordCalls) != 0 {
		t.Fatal("vault probe must not decrypt")
	}
}

func TestDuplicateProbeMissingNamesIsIndeterminate(t *testing.T) {
	svc := duplicateService(t, &MemoryCatalog{}, createVaultKey(t))
	outcome, err := NewDuplicateProbe(svc).DuplicateState(context.Background(), operations.Operation{Target: "project_a_copy"})
	if err != nil {
		t.Fatal(err)
	}
	if !outcome.Indeterminate {
		t.Fatalf("outcome = %#v", outcome)
	}
}
