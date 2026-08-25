package securefile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var (
	ErrNotRegular = errors.New("not a regular file")
	errNotRealDir = errors.New("not a real directory")
	errTrunc      = errors.New("destructive open flag")
)

// OpenRegular opens one regular file without trusting a path-only pre-check.
// Ancestor directories are inspected with Lstat so an intermediate symlink
// cannot retarget the open. The file identity is checked before and after
// opening so a final-component symlink or replacement fails closed.
func OpenRegular(path string, flag int, perm fs.FileMode) (*os.File, error) {
	if err := rejectDestructiveFlags(flag); err != nil {
		return nil, err
	}
	dir, name := filepath.Dir(path), filepath.Base(path)
	if name == "." || name == string(filepath.Separator) {
		return nil, ErrNotRegular
	}
	if err := verifyRealAncestors(dir); err != nil {
		return nil, err
	}
	root, err := openVerifiedRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	return openRegularInRoot(root, name, flag, perm)
}

// OpenRegularUnder opens path relative to a verified real jail directory.
// Intermediate components inside the jail are created or opened without
// following symlink directories. A relative path that escapes the jail fails closed.
func OpenRegularUnder(jail, path string, flag int, perm fs.FileMode) (*os.File, error) {
	if err := rejectDestructiveFlags(flag); err != nil {
		return nil, err
	}
	jail = filepath.Clean(jail)
	cleaned := filepath.Clean(path)
	rel, err := filepath.Rel(jail, cleaned)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, ErrNotRegular
	}
	if err := verifyRealAncestors(jail); err != nil {
		return nil, err
	}
	root, err := openVerifiedRoot(jail)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	dirRel := filepath.Dir(rel)
	if dirRel != "." {
		if err := mkdirAllInRoot(root, dirRel, 0o700); err != nil {
			return nil, err
		}
	}
	return openRegularInRoot(root, rel, flag, perm)
}

// EnsureRealDir creates path as a chain of real directories. Existing
// components must be directories and must not be symlinks. Missing
// components are created with os.Root relative to a verified parent.
func EnsureRealDir(path string, perm fs.FileMode) error {
	path = filepath.Clean(path)
	if path == "." || path == "" {
		return nil
	}
	var chain []string
	current := path
	for {
		chain = append(chain, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	for i := len(chain) - 1; i >= 0; i-- {
		component := chain[i]
		info, err := os.Lstat(component)
		if errors.Is(err, fs.ErrNotExist) {
			parent := filepath.Dir(component)
			if err := verifyRealDir(parent); err != nil {
				return err
			}
			root, err := openVerifiedRoot(parent)
			if err != nil {
				return err
			}
			mkErr := root.Mkdir(filepath.Base(component), perm)
			_ = root.Close()
			if mkErr != nil && !errors.Is(mkErr, fs.ErrExist) {
				return mkErr
			}
			if err := verifyRealDir(component); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if isSymlinkOrNotDir(info) {
			return errNotRealDir
		}
	}
	return nil
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
	dir := filepath.Dir(path)
	if err := verifyRealAncestors(dir); err != nil {
		return err
	}
	root, err := openVerifiedRoot(dir)
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

func rejectDestructiveFlags(flag int) error {
	if flag&os.O_TRUNC != 0 {
		return errTrunc
	}
	return nil
}

func verifyRealAncestors(path string) error {
	path = filepath.Clean(path)
	if path == "" {
		return errNotRealDir
	}
	var chain []string
	current := path
	for {
		chain = append(chain, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if err := verifyRealDir(chain[i]); err != nil {
			return err
		}
	}
	return nil
}

func verifyRealDir(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if isSymlinkOrNotDir(info) {
		return errNotRealDir
	}
	return nil
}

func openVerifiedRoot(dir string) (*os.Root, error) {
	before, err := os.Lstat(dir)
	if err != nil {
		return nil, err
	}
	if isSymlinkOrNotDir(before) {
		return nil, errNotRealDir
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	opened, err := root.Stat(".")
	if err != nil || !opened.IsDir() || !os.SameFile(before, opened) {
		_ = root.Close()
		return nil, errNotRealDir
	}
	return root, nil
}

func mkdirAllInRoot(root *os.Root, rel string, perm fs.FileMode) error {
	rel = filepath.Clean(rel)
	if rel == "." {
		return nil
	}
	if !filepath.IsLocal(rel) {
		return errNotRealDir
	}
	acc := ""
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		if acc == "" {
			acc = part
		} else {
			acc = filepath.Join(acc, part)
		}
		info, err := root.Lstat(acc)
		if errors.Is(err, fs.ErrNotExist) {
			if err := root.Mkdir(acc, perm); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if isSymlinkOrNotDir(info) {
			return errNotRealDir
		}
	}
	return nil
}

func openRegularInRoot(root *os.Root, rel string, flag int, perm fs.FileMode) (*os.File, error) {
	rel = filepath.Clean(rel)
	if rel == "." || !filepath.IsLocal(rel) {
		return nil, ErrNotRegular
	}
	dir, name := filepath.Dir(rel), filepath.Base(rel)
	if name == "." || name == string(filepath.Separator) {
		return nil, ErrNotRegular
	}
	openRoot := root
	if dir != "." {
		if err := verifyRootRealDir(root, dir); err != nil {
			return nil, err
		}
		nested, err := root.OpenRoot(dir)
		if err != nil {
			return nil, err
		}
		defer nested.Close()
		openRoot = nested
	}
	return openNamedRegular(openRoot, name, flag, perm)
}

func verifyRootRealDir(root *os.Root, rel string) error {
	rel = filepath.Clean(rel)
	if rel == "." {
		return nil
	}
	if !filepath.IsLocal(rel) {
		return errNotRealDir
	}
	acc := ""
	for _, part := range strings.Split(rel, string(filepath.Separator)) {
		if part == "" || part == "." {
			continue
		}
		if acc == "" {
			acc = part
		} else {
			acc = filepath.Join(acc, part)
		}
		info, err := root.Lstat(acc)
		if err != nil {
			return err
		}
		if isSymlinkOrNotDir(info) {
			return errNotRealDir
		}
	}
	return nil
}

func openNamedRegular(root *os.Root, name string, flag int, perm fs.FileMode) (*os.File, error) {
	if strings.ContainsRune(name, filepath.Separator) || name == "." || name == ".." {
		return nil, ErrNotRegular
	}
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

func isSymlinkOrNotDir(info fs.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0 || !info.IsDir()
}
