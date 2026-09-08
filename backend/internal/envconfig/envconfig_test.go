package envconfig

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValueUsesEnvironmentWithoutFile(t *testing.T) {
	t.Setenv("TEST_SECRET", "environment-value")
	t.Setenv("TEST_SECRET_FILE", "")

	value, err := Value("TEST_SECRET")
	require.NoError(t, err)
	require.Equal(t, "environment-value", value)
}

func TestValuePrefersFileAndTrimsLineEnding(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secretPath, []byte("file-value\r\n"), 0o600))
	t.Setenv("TEST_SECRET", "stale-environment-value")
	t.Setenv("TEST_SECRET_FILE", secretPath)

	value, err := Value("TEST_SECRET")
	require.NoError(t, err)
	require.Equal(t, "file-value", value)
}

func TestValueReportsMissingSecretFile(t *testing.T) {
	t.Setenv("TEST_SECRET_FILE", filepath.Join(t.TempDir(), "missing"))

	_, err := Value("TEST_SECRET")
	require.ErrorContains(t, err, "read TEST_SECRET_FILE")
}

func TestApplyFileOverridesReplacesExistingEnvironment(t *testing.T) {
	secretPath := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(secretPath, []byte("file-value\n"), 0o600))
	t.Setenv("TEST_SECRET", "stale-environment-value")
	t.Setenv("TEST_SECRET_FILE", secretPath)

	require.NoError(t, ApplyFileOverrides("TEST_SECRET"))
	require.Equal(t, "file-value", os.Getenv("TEST_SECRET"))
}
