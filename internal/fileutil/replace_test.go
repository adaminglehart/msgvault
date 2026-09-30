package fileutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSecureReplaceFileReplacesContentWithRequestedPerm(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	require.NoError(os.WriteFile(path, []byte("old"), 0o644))
	require.NoError(os.Chmod(path, 0o644))

	require.NoError(SecureReplaceFile(path, []byte("new"), 0o600))

	got, err := os.ReadFile(path)
	require.NoError(err)
	assert.Equal("new", string(got))
	entries, err := os.ReadDir(dir)
	require.NoError(err)
	require.Len(entries, 1, "the staged file must not be left behind")
	assert.Equal("token.json", entries[0].Name())
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(err)
		assert.Equal(os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestSecureReplacePrivateFilePublishesOwnerOnlyFile(t *testing.T) {
	require := require.New(t)
	path := filepath.Join(t.TempDir(), "token.json")
	require.NoError(SecureReplacePrivateFile(path, []byte("secret")))

	got, err := os.ReadFile(path)
	require.NoError(err)
	assert.Equal(t, "secret", string(got))
	file, err := os.Open(path)
	require.NoError(err)
	defer file.Close() //nolint:errcheck // read-only file
	assert.NoError(t, VerifyPrivateFile(file, 0o600))
}

func TestSecureReplacePrivateFileRefusesSymlinkTarget(t *testing.T) {
	require := require.New(t)
	dir := t.TempDir()
	target := filepath.Join(dir, "real.json")
	require.NoError(os.WriteFile(target, []byte("old"), 0o600))
	link := filepath.Join(dir, "token.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	require.Error(SecureReplacePrivateFile(link, []byte("new")))

	info, err := os.Lstat(link)
	require.NoError(err)
	assert.NotZero(t, info.Mode()&os.ModeSymlink, "the symlink must stay in place")
	got, err := os.ReadFile(target)
	require.NoError(err)
	assert.Equal(t, "old", string(got))
}

func TestSecureReplacePrivateFileKeepsPriorFileWhenVerifyFails(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "token.json")
	require.NoError(os.WriteFile(path, []byte("prior"), 0o600))
	previous := verifyPrivateStaged
	verifyPrivateStaged = func(*os.File, os.FileMode) error { return errors.New("not owner-only") }
	t.Cleanup(func() { verifyPrivateStaged = previous })

	err := SecureReplacePrivateFile(path, []byte("new"))

	require.ErrorContains(err, "verify replacement")
	got, readErr := os.ReadFile(path)
	require.NoError(readErr)
	assert.Equal("prior", string(got))
	entries, readErr := os.ReadDir(dir)
	require.NoError(readErr)
	require.Len(entries, 1, "the staged file must not be left behind")
	assert.Equal("token.json", entries[0].Name())
}
