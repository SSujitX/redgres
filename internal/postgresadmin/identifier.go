package postgresadmin

import (
	"regexp"
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
