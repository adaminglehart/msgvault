package fileutil

import (
	"fmt"
	"os"

	"go.kenn.io/kit/atomicfile"
)

// SecureReplaceFile atomically replaces path with data. The staged file gets
// perm through SecureChmod before any data is written, so an owner-only perm
// also gets the owner-only DACL on Windows. The staged file and its directory
// are fsynced. A symlink or junction at path is refused, not replaced.
//
// An error wrapping atomicfile.ErrPublished means data is already visible at
// path but a later step, such as the directory fsync, failed.
func SecureReplaceFile(path string, data []byte, perm os.FileMode) error {
	return secureReplace(path, data, perm, false)
}

// SecureReplacePrivateFile is SecureReplaceFile at mode 0600 that refuses to
// publish unless VerifyPrivateFile accepts the staged file, so a DACL that
// could not be applied fails the write instead of logging a warning.
func SecureReplacePrivateFile(path string, data []byte) error {
	return secureReplace(path, data, 0o600, true)
}

// verifyPrivateStaged is a test seam for the staged-file check.
var verifyPrivateStaged = VerifyPrivateFile

func secureReplace(path string, data []byte, perm os.FileMode, verify bool) error {
	// WithPrivate also grants SYSTEM and Administrators access on Windows;
	// retain SecureChmod's current-user-only DACL for owner-only modes.
	file, err := atomicfile.Create(path, atomicfile.WithPerm(perm))
	if err != nil {
		return fmt.Errorf("stage replacement: %w", err)
	}
	defer func() { _ = file.Abort() }()
	if err := SecureChmod(file.TempName(), perm); err != nil {
		return fmt.Errorf("secure replacement: %w", err)
	}
	if verify {
		if err := verifyStaged(file.TempName(), perm); err != nil {
			return fmt.Errorf("verify replacement: %w", err)
		}
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write replacement: %w", err)
	}
	if err := file.Commit(); err != nil {
		return fmt.Errorf("publish replacement: %w", err)
	}
	return nil
}

func verifyStaged(path string, perm os.FileMode) error {
	staged, err := os.Open(path) // #nosec G304 -- path is the staging file atomicfile just created.
	if err != nil {
		return err
	}
	defer staged.Close() //nolint:errcheck // read-only file
	return verifyPrivateStaged(staged, perm)
}
