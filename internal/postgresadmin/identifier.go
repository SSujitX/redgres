package postgresadmin

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const maxIdentifierLength = 63

var identifierPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func ValidateIdentifier(name string) error {
	if name == "" || utf8.RuneCountInString(name) > maxIdentifierLength {
		return ErrInvalidIdentifier
	}
	if !identifierPattern.MatchString(name) {
		return ErrInvalidIdentifier
	}
	return nil
}

func QuoteIdentifier(name string) (string, error) {
	if err := ValidateIdentifier(name); err != nil {
		return "", err
	}
	return pgx.Identifier{name}.Sanitize(), nil
}

// QuoteCatalogIdentifier quotes a name taken from PostgreSQL catalogs.
// HTTP path/body names stay on ValidateIdentifier + QuoteIdentifier.
func QuoteCatalogIdentifier(name string) (string, error) {
	if name == "" || strings.ContainsRune(name, 0) {
		return "", ErrInvalidIdentifier
	}
	return pgx.Identifier{name}.Sanitize(), nil
}
