package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/SSujitX/redgres/internal/auth"
	"github.com/SSujitX/redgres/internal/config"
	"github.com/SSujitX/redgres/internal/database"
	"github.com/SSujitX/redgres/migrations"
	"golang.org/x/term"
)

var readPasswordPair = defaultReadPasswordPair

func createOwner(args []string) error {
	if err := config.LoadDevelopmentDotEnv(args); err != nil {
		return err
	}

	fs := flag.NewFlagSet("create-owner", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	username := fs.String("username", "", "owner username")
	sqlitePath := fs.String("sqlite-path", envDefault("REDGRES_SQLITE_PATH", config.DefaultSQLitePath), "SQLite database path")
	replace := fs.Bool("replace", false, "replace the existing owner")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := auth.ValidateUsername(*username); err != nil {
		return err
	}

	pw1, pw2, err := readPasswordPair()
	if err != nil {
		return err
	}
	if pw1 != pw2 {
		return auth.ErrPasswordConfirm
	}

	db, err := database.Open(*sqlitePath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := database.Migrate(db, migrations.FS); err != nil {
		return err
	}

	if _, err := auth.CreateOrReplaceOwner(db, *username, pw1, *replace); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "owner created")
	return nil
}

func defaultReadPasswordPair() (string, string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return "", "", errors.New("password must be entered interactively from a terminal")
	}
	fmt.Fprint(os.Stderr, "Password: ")
	b1, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", "", err
	}
	fmt.Fprint(os.Stderr, "Confirm password: ")
	b2, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", "", err
	}
	return string(b1), string(b2), nil
}

func envDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
