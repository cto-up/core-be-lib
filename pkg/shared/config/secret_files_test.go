package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"ctoup.com/coreapp/pkg/shared/config"
)

func writeSecret(t *testing.T, dir, name, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o400))
}

func TestLoadSecretFilesSetsUnsetVariablesFromFiles(t *testing.T) {
	dir := t.TempDir()
	writeSecret(t, dir, "CORE_TEST_SECRET_A", "s3cr3t\n")
	writeSecret(t, dir, "CORE_TEST_SECRET_EMPTY", "")
	t.Setenv("CORE_TEST_SECRET_A", "")
	require.NoError(t, os.Unsetenv("CORE_TEST_SECRET_A"))
	t.Setenv("CORE_TEST_SECRET_EMPTY", "")
	require.NoError(t, os.Unsetenv("CORE_TEST_SECRET_EMPTY"))

	set, err := config.LoadSecretFiles(dir)

	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"CORE_TEST_SECRET_A", "CORE_TEST_SECRET_EMPTY"}, set)
	assert.Equal(t, "s3cr3t", os.Getenv("CORE_TEST_SECRET_A"))
	v, ok := os.LookupEnv("CORE_TEST_SECRET_EMPTY")
	assert.True(t, ok, "an empty file still sets the variable, as an empty compose value did")
	assert.Empty(t, v)
}

func TestLoadSecretFilesNeverOverridesTheEnvironment(t *testing.T) {
	dir := t.TempDir()
	writeSecret(t, dir, "CORE_TEST_SECRET_B", "from-file")
	t.Setenv("CORE_TEST_SECRET_B", "from-env")

	set, err := config.LoadSecretFiles(dir)

	require.NoError(t, err)
	assert.Empty(t, set)
	assert.Equal(t, "from-env", os.Getenv("CORE_TEST_SECRET_B"))
}

func TestLoadSecretFilesSkipsNamesThatAreNotVariables(t *testing.T) {
	dir := t.TempDir()
	writeSecret(t, dir, "kratos.yml", "dsn: x")
	require.NoError(t, os.Mkdir(filepath.Join(dir, "CORE_TEST_SECRET_DIR"), 0o700))

	set, err := config.LoadSecretFiles(dir)

	require.NoError(t, err)
	assert.Empty(t, set)
	_, ok := os.LookupEnv("CORE_TEST_SECRET_DIR")
	assert.False(t, ok)
}

func TestLoadSecretFilesWithoutTheDirectoryIsANoOp(t *testing.T) {
	set, err := config.LoadSecretFiles(filepath.Join(t.TempDir(), "absent"))

	require.NoError(t, err)
	assert.Empty(t, set)
}

func TestLoadSecretFilesFailsOnAnUnreadableSecret(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a 0000 file")
	}
	dir := t.TempDir()
	writeSecret(t, dir, "CORE_TEST_SECRET_C", "x")
	require.NoError(t, os.Chmod(filepath.Join(dir, "CORE_TEST_SECRET_C"), 0))
	t.Setenv("CORE_TEST_SECRET_C", "")
	require.NoError(t, os.Unsetenv("CORE_TEST_SECRET_C"))

	_, err := config.LoadSecretFiles(dir)

	assert.ErrorContains(t, err, "CORE_TEST_SECRET_C")
}
