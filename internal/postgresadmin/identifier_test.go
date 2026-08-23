package postgresadmin

import (
	"strings"
	"testing"
)

func TestValidateIdentifier(t *testing.T) {
	valid := []string{"project_a", "A", "_x", "db1"}
	for _, name := range valid {
		if err := ValidateIdentifier(name); err != nil {
			t.Fatalf("%q: %v", name, err)
		}
	}
	invalid := []string{"", "1db", "bad-name", "quote\"x", "1;drop", "café", strings.Repeat("a", 64)}
	for _, name := range invalid {
		if err := ValidateIdentifier(name); err == nil {
			t.Fatalf("%q: expected invalid", name)
		}
	}
}

func TestQuoteIdentifierUsesPgx(t *testing.T) {
	got, err := QuoteIdentifier("project_a")
	if err != nil {
		t.Fatal(err)
	}
	if got != `"project_a"` {
		t.Fatalf("got %s", got)
	}
	if _, err := QuoteIdentifier(`bad-name`); err == nil {
		t.Fatal("expected invalid")
	}
}
