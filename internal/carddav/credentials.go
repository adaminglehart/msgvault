package carddav

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"go.kenn.io/kit/atomicfile"
	"go.kenn.io/msgvault/internal/fileutil"
)

const cardDAVTokenFilename = "carddav.json" // #nosec G101 -- This is a credential filename, not a credential value.

const maximumCredentialFileBytes = 1 << 20

var ErrCredentialNotBound = errors.New("CardDAV credential is not bound to a connection")

// Credential binds a password to the exact durable connection it may
// authenticate. The generation closes the crash window between publishing the
// filesystem pair and replacing the discovery snapshot: startup fails closed
// until config, token, and database all describe the same connection.
type Credential struct {
	Password             string `json:"password,omitempty"`
	Google               bool   `json:"google,omitzero"`
	OAuthApp             string `json:"oauth_app,omitempty"`
	BaseURL              string `json:"base_url,omitempty"`
	Username             string `json:"username,omitempty"`
	ConnectionGeneration int64  `json:"connection_generation,omitzero"`
}

// CredentialFileSnapshot retains the exact published credential bytes long
// enough for an account-save rollback. It deliberately does not decode or
// verify the file, so an explicit password can repair a malformed credential.
type CredentialFileSnapshot struct {
	contents []byte
	exists   bool
}

// SavePassword atomically replaces the CardDAV token file. The password is
// deliberately kept out of config and durable database records.
func SavePassword(tokenDir, password string) error {
	return saveCredential(tokenDir, Credential{Password: password})
}

// SaveCredential atomically publishes an identity-bound CardDAV credential.
func SaveCredential(tokenDir string, credential Credential) error {
	if (credential.Password == "" && !credential.Google) || credential.BaseURL == "" || credential.Username == "" || credential.ConnectionGeneration <= 0 {
		return errors.New("CardDAV credential requires a connection identity")
	}
	return saveCredential(tokenDir, credential)
}

// CaptureCredentialFile snapshots the current credential without requiring it
// to be valid. A missing credential is a valid empty snapshot.
func CaptureCredentialFile(tokenDir string) (CredentialFileSnapshot, error) {
	path := filepath.Join(tokenDir, cardDAVTokenFilename)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return CredentialFileSnapshot{}, nil
	}
	if err != nil {
		return CredentialFileSnapshot{}, fmt.Errorf("open CardDAV token file for rollback: %w", err)
	}
	defer file.Close() //nolint:errcheck // read-only file
	contents, err := io.ReadAll(io.LimitReader(file, maximumCredentialFileBytes+1))
	if err != nil {
		return CredentialFileSnapshot{}, fmt.Errorf("read CardDAV token file for rollback: %w", err)
	}
	if len(contents) > maximumCredentialFileBytes {
		return CredentialFileSnapshot{}, errors.New("CardDAV token file exceeds rollback size limit")
	}
	return CredentialFileSnapshot{contents: contents, exists: true}, nil
}

// Restore atomically restores the captured credential bytes, or removes a
// newly published credential when the snapshot represented a missing file.
func (s CredentialFileSnapshot) Restore(tokenDir string) error {
	if !s.exists {
		return RemoveCredential(tokenDir)
	}
	return saveCredentialBytes(tokenDir, s.contents)
}

// RemoveCredential removes a published CardDAV credential. Missing files are
// already the desired state and therefore succeed.
func RemoveCredential(tokenDir string) error {
	err := os.Remove(filepath.Join(tokenDir, cardDAVTokenFilename))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove CardDAV token file: %w", err)
	}
	return nil
}

func saveCredential(tokenDir string, credential Credential) error {
	var encoded bytes.Buffer
	// #nosec G117 -- The credential is intentionally marshaled only into the private token-file buffer.
	if err := json.MarshalWrite(&encoded, credential, json.Deterministic(true)); err != nil {
		return fmt.Errorf("encode CardDAV token file: %w", err)
	}
	return saveCredentialBytes(tokenDir, encoded.Bytes())
}

func saveCredentialBytes(tokenDir string, contents []byte) error {
	if err := fileutil.SecureMkdirAll(tokenDir, 0o700); err != nil {
		return fmt.Errorf("secure CardDAV token directory: %w", err)
	}
	if err := fileutil.SecureChmod(tokenDir, 0o700); err != nil {
		return fmt.Errorf("secure CardDAV token directory: %w", err)
	}
	err := fileutil.SecureReplacePrivateFile(filepath.Join(tokenDir, cardDAVTokenFilename), contents)
	// Publication is the commit point; a later directory fsync failure keeps the new credential.
	if err != nil && !errors.Is(err, atomicfile.ErrPublished) {
		return fmt.Errorf("replace CardDAV token file: %w", err)
	}
	return nil
}

// LoadPassword reads the private CardDAV token file and rejects files exposed
// to group or other users. Errors never include the token contents.
func LoadPassword(tokenDir string) (string, error) {
	credential, err := loadCredential(tokenDir)
	if err != nil {
		return "", err
	}
	return credential.Password, nil
}

// LoadCredential reads an identity-bound private credential. Legacy
// password-only files remain readable through LoadPassword, but are rejected
// here so the daemon can never pair them with an arbitrary configured origin.
func LoadCredential(tokenDir string) (Credential, error) {
	credential, err := loadCredential(tokenDir)
	if err != nil {
		return Credential{}, err
	}
	if credential.BaseURL == "" || credential.Username == "" || credential.ConnectionGeneration <= 0 {
		return Credential{}, ErrCredentialNotBound
	}
	return credential, nil
}

// LoadLegacyPassword reads only the historical password-only token shape.
// Partially bound records fail closed instead of being silently rebound.
func LoadLegacyPassword(tokenDir string) (string, error) {
	credential, err := loadCredential(tokenDir)
	if err != nil {
		return "", err
	}
	if credential.BaseURL != "" || credential.Username != "" || credential.ConnectionGeneration != 0 {
		return "", errors.New("CardDAV credential is not a legacy password-only record")
	}
	return credential.Password, nil
}

func loadCredential(tokenDir string) (Credential, error) {
	path := filepath.Join(tokenDir, cardDAVTokenFilename)
	file, err := os.Open(path)
	if err != nil {
		return Credential{}, fmt.Errorf("open CardDAV token file: %w", err)
	}
	defer file.Close() //nolint:errcheck // read-only file
	if err := fileutil.VerifyPrivateFile(file, 0o600); err != nil {
		return Credential{}, fmt.Errorf("verify CardDAV token file permissions: %w", err)
	}
	decoder := jsontext.NewDecoder(io.LimitReader(file, maximumCredentialFileBytes), json.RejectUnknownMembers(true))

	var saved Credential
	if err := json.UnmarshalDecode(decoder, &saved); err != nil {
		return Credential{}, fmt.Errorf("decode CardDAV token file: %w", err)
	}
	if saved.Google && saved.Password != "" {
		return Credential{}, errors.New("a Google CardDAV credential cannot contain a password")
	}
	if saved.Password == "" && !saved.Google {
		return Credential{}, errors.New("CardDAV token file contains an empty password")
	}
	return saved, nil
}
