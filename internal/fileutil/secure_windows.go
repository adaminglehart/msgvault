//go:build windows

package fileutil

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1FF

// isOwnerOnly returns true if the permission mode grants nothing to group or other.
func isOwnerOnly(perm os.FileMode) bool {
	return perm&0077 == 0
}

// restrictToCurrentUser sets a DACL on path that grants GENERIC_ALL only to
// the current user and blocks inherited ACEs. For directories, the DACL
// includes CONTAINER_INHERIT_ACE | OBJECT_INHERIT_ACE so that child files
// and subdirectories automatically inherit the restriction. Errors are
// returned to the caller; the file was already created with the requested
// Unix mode, so callers may treat DACL failures as non-fatal warnings.
func restrictToCurrentUser(path string) error {
	token := windows.GetCurrentProcessToken()

	user, err := token.GetTokenUser()
	if err != nil {
		return fmt.Errorf("fileutil: get current user SID for %s: %w", path, err)
	}

	trustee := windows.TrusteeValueFromSID(user.User.Sid)

	// For directories, enable inheritance so children get the same restriction.
	// For files, NO_INHERITANCE is correct (files don't have children).
	var inherit uint32 = windows.NO_INHERITANCE
	info, statErr := os.Stat(path)
	if statErr == nil && info.IsDir() {
		inherit = windows.CONTAINER_INHERIT_ACE | windows.OBJECT_INHERIT_ACE
	}

	ea := []windows.EXPLICIT_ACCESS{
		{
			AccessPermissions: windows.GENERIC_ALL,
			AccessMode:        windows.SET_ACCESS,
			Inheritance:       inherit,
			Trustee: windows.TRUSTEE{
				TrusteeForm:  windows.TRUSTEE_IS_SID,
				TrusteeType:  windows.TRUSTEE_IS_USER,
				TrusteeValue: trustee,
			},
		},
	}

	acl, err := windows.ACLFromEntries(ea, nil)
	if err != nil {
		return fmt.Errorf("fileutil: build ACL for %s: %w", path, err)
	}

	secInfo := windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.SECURITY_INFORMATION(secInfo),
		nil, // owner SID (unchanged)
		nil, // group SID (unchanged)
		acl, // DACL
		nil, // SACL (unchanged)
	)
	if err != nil {
		return fmt.Errorf("fileutil: set DACL on %s: %w", path, err)
	}
	return nil
}

// SecureWriteFile writes data to the named file, creating it if necessary.
// For owner-only modes, a DACL restricting access to the current user is applied.
// DACL failures are logged as warnings but do not fail the write.
func SecureWriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.WriteFile(path, data, perm); err != nil {
		return err
	}
	if isOwnerOnly(perm) {
		if err := restrictToCurrentUser(path); err != nil {
			slog.Warn("fileutil: best-effort DACL failed", "path", path, "err", err)
		}
	}
	return nil
}

// SecureMkdirAll creates a directory path and all parents that do not yet exist.
// For owner-only modes, a DACL restricting access to the current user is applied
// to the leaf directory and every intermediate directory that was created.
func SecureMkdirAll(path string, perm os.FileMode) error {
	// Determine which directories already exist before creating.
	var toSecure []string
	if isOwnerOnly(perm) {
		p := filepath.Clean(path)
		for p != "" && p != "." && p != string(filepath.Separator) {
			if _, err := os.Stat(p); err == nil {
				break // already exists, stop climbing
			}
			toSecure = append(toSecure, p)
			parent := filepath.Dir(p)
			if parent == p {
				break
			}
			p = parent
		}
	}

	if err := os.MkdirAll(path, perm); err != nil {
		return err
	}

	// Secure all newly created directories (leaf-first order, but order doesn't matter).
	for _, dir := range toSecure {
		if err := restrictToCurrentUser(dir); err != nil {
			slog.Warn("fileutil: best-effort DACL failed", "path", dir, "err", err)
		}
	}
	return nil
}

// SecureChmod changes the mode of the named file.
// For owner-only modes, a DACL restricting access to the current user is applied.
// DACL failures are logged as warnings but do not fail the chmod.
func SecureChmod(path string, perm os.FileMode) error {
	if err := os.Chmod(path, perm); err != nil {
		return err
	}
	if isOwnerOnly(perm) {
		if err := restrictToCurrentUser(path); err != nil {
			slog.Warn("fileutil: best-effort DACL failed", "path", path, "err", err)
		}
	}
	return nil
}

// SecureOpenFile opens the named file with specified flag and permissions.
// For owner-only modes when O_CREATE is set, a DACL restricting access to
// the current user is applied — regardless of whether the file already existed.
// This is intentional: all callers write sensitive data (email content,
// attachments) that should be owner-only.
//
// Note: on Windows there is a small TOCTOU window between file creation and
// DACL application because SetNamedSecurityInfo operates by path after the
// file is already open. The window is very brief and exploitation would
// require local access, so this is acceptable for the threat model.
// DACL failures are logged as warnings but do not fail the open.
func SecureOpenFile(path string, flag int, perm os.FileMode) (*os.File, error) {
	f, err := os.OpenFile(path, flag, perm)
	if err != nil {
		return nil, err
	}
	if isOwnerOnly(perm) && (flag&os.O_CREATE != 0) {
		if err := restrictToCurrentUser(path); err != nil {
			slog.Warn("fileutil: best-effort DACL failed", "path", path, "err", err)
		}
	}
	return f, nil
}

// VerifyPrivateFile reports an error unless the open file is owned by the
// current user (or an Administrators group the user belongs to) and its
// protected DACL holds exactly one non-inherited full-control entry for the
// current user. perm is unused on Windows; the DACL is the access rule.
func VerifyPrivateFile(file *os.File, _ os.FileMode) error {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return fmt.Errorf("get current user SID: %w", err)
	}
	descriptor, err := windows.GetSecurityInfo(windows.Handle(file.Fd()), windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read DACL: %w", err)
	}
	owner, _, err := descriptor.Owner()
	if err != nil || owner == nil {
		return fmt.Errorf("read owner: %w", err)
	}
	if err := verifyOwner(owner, user.User.Sid); err != nil {
		return err
	}
	control, _, err := descriptor.Control()
	if err != nil {
		return fmt.Errorf("read DACL control: %w", err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		return errors.New("DACL permits inherited access")
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		return fmt.Errorf("read DACL entries: %w", err)
	}
	if dacl.AceCount != 1 {
		return errors.New("DACL must contain exactly one access entry")
	}
	var ace *windows.ACCESS_ALLOWED_ACE
	if err := windows.GetAce(dacl, 0, &ace); err != nil {
		return fmt.Errorf("read owner ACE: %w", err)
	}
	if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE ||
		(ace.Mask != windows.GENERIC_ALL && ace.Mask != fileAllAccess) {
		return errors.New("DACL does not grant exactly full control")
	}
	if ace.Header.AceFlags&windows.INHERITED_ACE != 0 {
		return errors.New("DACL contains inherited access")
	}
	// #nosec G103 -- GetAce supplies an access-allowed ACE whose SidStart is the first word of its contiguous SID.
	aceSID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
	if !aceSID.Equals(user.User.Sid) {
		return errors.New("DACL grants a principal other than the current user")
	}
	return nil
}

func verifyOwner(owner, user *windows.SID) error {
	if owner.Equals(user) {
		return nil
	}
	if owner.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
		member, err := windows.Token(0).IsMember(owner)
		if err != nil {
			return fmt.Errorf("check Administrators membership: %w", err)
		}
		if member {
			return nil
		}
	}
	return errors.New("file owner is not the current user")
}
