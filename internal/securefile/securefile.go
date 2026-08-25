package securefile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

var ErrNotRegular = errors.New("not a regular file")

// OpenRegular opens one regular file without trusting a path-only pre-check.
// The file identity is checked before and after opening so a final-component
// symlink or replacement fails closed.
func OpenRegular(path string, flag int, perm fs.FileMode) (*os.File, error) {
	dir, name := filepath.Dir(path), filepath.Base(path)
	if name == "." || name == string(filepath.Separator) {
		return nil, ErrNotRegular
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	before, err := root.Lstat(name)
	switch {
	case err == nil:
		if !before.Mode().IsRegular() {
			return nil, ErrNotRegular
		}
		flag &^= os.O_CREATE
	case errors.Is(err, fs.ErrNotExist) && flag&os.O_CREATE != 0:
		before = nil
		flag |= os.O_EXCL
	case err != nil:
		return nil, err
	}

	file, err := root.OpenFile(name, flag, perm)
	if err != nil {
		return nil, err
	}
	closeOnError := func(openErr error) (*os.File, error) {
		_ = file.Close()
		return nil, openErr
	}

	opened, err := file.Stat()
	if err != nil {
		return closeOnError(err)
	}
	if !opened.Mode().IsRegular() {
		return closeOnError(ErrNotRegular)
	}
	after, err := root.Lstat(name)
	if err != nil {
		return closeOnError(err)
	}
	if !after.Mode().IsRegular() || !os.SameFile(opened, after) {
		return closeOnError(ErrNotRegular)
	}
	if before != nil && !os.SameFile(before, opened) {
		return closeOnError(ErrNotRegular)
	}
	return file, nil
}

func ReadRegular(path string, validateMode func(fs.FileMode) error) ([]byte, error) {
	file, err := OpenRegular(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if validateMode != nil {
		if err := validateMode(info.Mode()); err != nil {
			return nil, err
		}
	}
	raw, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

func VerifyRegularPath(path string, file *os.File) error {
	if file == nil {
		return fmt.Errorf("verify regular file: %w", ErrNotRegular)
	}
	opened, err := file.Stat()
	if err != nil {
		return err
	}
	if !opened.Mode().IsRegular() {
		return ErrNotRegular
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer root.Close()
	current, err := root.Lstat(filepath.Base(path))
	if err != nil {
		return err
	}
	if !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		return ErrNotRegular
	}
	return nil
}
