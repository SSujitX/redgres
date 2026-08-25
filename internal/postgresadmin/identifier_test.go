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

func TestQuoteCatalogIdentifier(t *testing.T) {
	got, err := QuoteCatalogIdentifier("Order")
	if err != nil {
		t.Fatal(err)
	}
	if got != `"Order"` {
		t.Fatalf("got %s", got)
	}
	hyphen, err := QuoteCatalogIdentifier("user-data")
	if err != nil {
		t.Fatal(err)
	}
	if hyphen != `"user-data"` {
		t.Fatalf("got %s", hyphen)
	}
	quoted, err := QuoteCatalogIdentifier(`quote"x`)
	if err != nil {
		t.Fatal(err)
	}
	if quoted != `"quote""x"` {
		t.Fatalf("got %s", quoted)
	}
	if _, err := QuoteCatalogIdentifier(""); err == nil {
		t.Fatal("empty must fail closed")
	}
	if _, err := QuoteCatalogIdentifier("a\x00b"); err == nil {
		t.Fatal("NUL must fail closed")
	}
	if _, err := QuoteIdentifier("user-data"); err == nil {
		t.Fatal("HTTP QuoteIdentifier must still reject hyphenated names")
	}
}
