package carddav

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.kenn.io/msgvault/internal/fileutil"
)

func TestCardDAVCredentialsRoundTripInPrivateTokenFile(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	home := t.TempDir()
	require.NoError(SavePassword(testCredentialTokenDir(home), "first-secret"))
	require.NoError(SavePassword(testCredentialTokenDir(home), "replacement-secret"))

	password, err := LoadPassword(testCredentialTokenDir(home))
	require.NoError(err)
	assert.Equal("replacement-secret", password)
	path := filepath.Join(home, "tokens", "carddav.json")
	file, err := os.Open(path)
	require.NoError(err)
	defer file.Close() //nolint:errcheck // read-only file
	require.NoError(fileutil.VerifyPrivateFile(file, 0o600))
	if runtime.GOOS == "windows" {
		return
	}

	info, err := os.Stat(path)
	require.NoError(err)
	assert.Equal(os.FileMode(0o600), info.Mode().Perm())
	tokensInfo, err := os.Stat(filepath.Dir(path))
	require.NoError(err)
	assert.Zero(tokensInfo.Mode().Perm() & 0o077)
}

func TestCardDAVCredentialBindsPasswordToConnectionIdentity(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	home := t.TempDir()
	want := Credential{
		Password:             "synthetic-secret",
		BaseURL:              "https://contacts.example/dav",
		Username:             "alice",
		ConnectionGeneration: 3,
	}
	require.NoError(SaveCredential(testCredentialTokenDir(home), want))

	got, err := LoadCredential(testCredentialTokenDir(home))
	require.NoError(err)
	assert.Equal(want, got)
	assert.NotContains(string(mustReadCredentialFile(t, testCredentialTokenDir(home))), "connection_generation\":0")
}

func TestCardDAVCredentialRejectsLegacyUnboundPassword(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, SavePassword(testCredentialTokenDir(home), "synthetic-secret"))

	_, err := LoadCredential(testCredentialTokenDir(home))
	require.ErrorContains(t, err, "not bound to a connection")
}

func testCredentialTokenDir(home string) string {
	return filepath.Join(home, "tokens")
}

func mustReadCredentialFile(t *testing.T, tokenDir string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(tokenDir, cardDAVTokenFilename))
	require.NoError(t, err)
	return content
}

func TestCardDAVCredentialsRejectExposedTokenFile(t *testing.T) {
	require := require.New(t)

	home := t.TempDir()
	require.NoError(os.MkdirAll(filepath.Join(home, "tokens"), 0o700))
	path := filepath.Join(home, "tokens", "carddav.json")
	require.NoError(os.WriteFile(path, []byte(`{"password":"secret"}`), 0o644))
	// The retained Linux verifier runs with a private umask, so WriteFile can
	// narrow the requested mode. Chmod establishes the exposed fixture exactly.
	require.NoError(os.Chmod(path, 0o644))

	_, err := LoadPassword(testCredentialTokenDir(home))
	require.ErrorContains(err, "permissions")
	assert.NotContains(t, err.Error(), "secret")
}

func TestCardDAVCredentialsRequireUnixTokenMode0600(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("Windows token privacy is verified from the DACL")
	}
	for _, mode := range []os.FileMode{0o400, 0o640} {
		home := t.TempDir()
		require.NoError(t, SavePassword(testCredentialTokenDir(home), "secret"))
		path := filepath.Join(home, "tokens", cardDAVTokenFilename)
		require.NoError(t, os.Chmod(path, mode))

		_, err := LoadPassword(testCredentialTokenDir(home))
		require.ErrorContains(t, err, "permissions", "mode %#o", mode)
	}
}
