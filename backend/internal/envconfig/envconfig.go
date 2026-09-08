// Package envconfig resolves configuration from environment variables and
// Docker-style secret files.
package envconfig

import (
	"fmt"
	"os"
	"strings"
)

// Value returns key_FILE when it is configured, otherwise key. File-backed
// values deliberately take precedence so a stale injected environment value
// cannot override a Docker secret.
func Value(key string) (string, error) {
	secretPath := strings.TrimSpace(os.Getenv(key + "_FILE"))
	if secretPath == "" {
		return os.Getenv(key), nil
	}

	value, err := os.ReadFile(secretPath)
	if err != nil {
		return "", fmt.Errorf("read %s_FILE: %w", key, err)
	}

	return strings.TrimRight(string(value), "\r\n"), nil
}

// ApplyFileOverrides resolves each key and places file-backed values in the
// process environment. This supports libraries such as OpenTelemetry that read
// configuration directly from os.Environ.
func ApplyFileOverrides(keys ...string) error {
	for _, key := range keys {
		if strings.TrimSpace(os.Getenv(key+"_FILE")) == "" {
			continue
		}

		value, err := Value(key)
		if err != nil {
			return err
		}
		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("set %s from %s_FILE: %w", key, key, err)
		}
	}

	return nil
}
