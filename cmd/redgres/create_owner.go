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

var generateOwnerPassword = auth.GeneratePassword

var openOwnerTTY = func() (*os.File, error) {
	return os.OpenFile("/dev/tty", os.O_WRONLY, 0)
}

func createOwner(args []string) error {
	if err := config.LoadDevelopmentDotEnv(args); err != nil {
		return err
	}

	fs := flag.NewFlagSet("create-owner", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	username := fs.String("username", "", "owner username")
	sqlitePath := fs.String("sqlite-path", envDefault("REDGRES_SQLITE_PATH", config.DefaultSQLitePath), "SQLite database path")
	replace := fs.Bool("replace", false, "replace the existing owner")
	generate := fs.Bool("generate", false, "generate a strong password and print it once to the controlling terminal")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := auth.ValidateUsername(*username); err != nil {
		return err
	}

	var password string
	var generatedTTY *os.File
	if *generate {
		tty, err := openOwnerTTY()
		if err != nil {
			return errors.New("create-owner --generate requires a controlling terminal")
		}
		generatedTTY = tty
		defer tty.Close()
		password, err = generateOwnerPassword()
		if err != nil {
			return err
		}
	} else {
		pw1, pw2, err := readPasswordPair()
		if err != nil {
			return err
		}
		if pw1 != pw2 {
			return auth.ErrPasswordConfirm
		}
		password = pw1
	}

	db, err := database.Open(*sqlitePath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := database.Migrate(db, migrations.FS); err != nil {
		return err
	}

	if _, err := auth.CreateOrReplaceOwner(db, *username, password, *replace); err != nil {
		return err
	}
	if generatedTTY != nil {
		if _, err := fmt.Fprintf(generatedTTY, "Generated owner password: %s\n", password); err != nil {
			return fmt.Errorf("owner created but the generated password could not be displayed; rerun create-owner --generate --replace: %w", err)
		}
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
