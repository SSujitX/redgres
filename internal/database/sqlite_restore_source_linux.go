package database

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"

	"golang.org/x/sys/unix"
)

func prepareSQLiteRestoreSource(ctx context.Context, _ string, pinned *os.File) (_ *sqliteRestoreSource, finalErr error) {
	info, err := pinned.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxSQLiteRestoreSnapshotBytes {
		return nil, fmt.Errorf("invalid sqlite restore source size")
	}
	fd, err := unix.MemfdCreate("redgres-sqlite-restore", unix.MFD_CLOEXEC|unix.MFD_ALLOW_SEALING)
	if err != nil {
		return nil, err
	}
	sealed := os.NewFile(uintptr(fd), "redgres-sqlite-restore")
	defer func() {
		if finalErr != nil {
			_ = sealed.Close()
		}
	}()
	if _, err := pinned.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	size, digest, err := copyBoundedSQLiteRestoreSource(ctx, sealed, pinned, maxSQLiteRestoreSnapshotBytes)
	if err != nil {
		return nil, err
	}
	if err := sealAndVerifySQLiteRestoreSource(ctx, sealed, size, digest); err != nil {
		return nil, err
	}
	return &sqliteRestoreSource{
		uri:       fmt.Sprintf("file:/proc/self/fd/%d?mode=ro", sealed.Fd()),
		size:      size,
		digest:    digest,
		closeFunc: sealed.Close,
	}, nil
}

func copyBoundedSQLiteRestoreSource(
	ctx context.Context,
	destination io.Writer,
	source io.Reader,
	maxBytes int64,
) (int64, string, error) {
	hash := sha256.New()
	buffer := make([]byte, 128*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return 0, "", err
		}
		n, readErr := source.Read(buffer)
		if n > 0 {
			if size+int64(n) > maxBytes {
				return 0, "", fmt.Errorf("sqlite restore source exceeds size limit")
			}
			written, writeErr := destination.Write(buffer[:n])
			if writeErr != nil {
				return 0, "", writeErr
			}
			if written != n {
				return 0, "", io.ErrShortWrite
			}
			_, _ = hash.Write(buffer[:n])
			size += int64(n)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, "", readErr
		}
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}

// requiredSQLiteRestoreSeals are the memfd seals applied to the restore
// source. F_SEAL_WRITE is deliberately omitted: modernc.org/sqlite opens the
// source connection read-write even for mode=ro, and a write seal makes that
// open fail with EPERM. The source is only ever read (mode=ro) and the memfd
// descriptor is process-private, so GROW/SHRINK/SEAL still prevent resize and
// seal-set changes while remaining openable.
const requiredSQLiteRestoreSeals = unix.F_SEAL_GROW | unix.F_SEAL_SHRINK | unix.F_SEAL_SEAL

func sealAndVerifySQLiteRestoreSource(ctx context.Context, file *os.File, expectedSize int64, expectedDigest string) error {
	if _, err := unix.FcntlInt(file.Fd(), unix.F_ADD_SEALS, requiredSQLiteRestoreSeals); err != nil {
		return err
	}
	seals, err := unix.FcntlInt(file.Fd(), unix.F_GET_SEALS, 0)
	if err != nil {
		return err
	}
	if seals&requiredSQLiteRestoreSeals != requiredSQLiteRestoreSeals {
		return fmt.Errorf("sqlite restore source seals are incomplete")
	}
	size, digest, err := hashSealedSQLiteRestoreSource(ctx, file)
	if err != nil {
		return err
	}
	if size != expectedSize || digest != expectedDigest {
		return fmt.Errorf("sealed sqlite restore source does not match pinned bytes")
	}
	return nil
}

func hashSealedSQLiteRestoreSource(ctx context.Context, file *os.File) (int64, string, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, "", err
	}
	hash := sha256.New()
	buffer := make([]byte, 128*1024)
	var size int64
	for {
		if err := ctx.Err(); err != nil {
			return 0, "", err
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
			size += int64(n)
			if size > maxSQLiteRestoreSnapshotBytes {
				return 0, "", fmt.Errorf("sealed sqlite restore source exceeds size limit")
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return 0, "", readErr
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, "", err
	}
	return size, hex.EncodeToString(hash.Sum(nil)), nil
}
