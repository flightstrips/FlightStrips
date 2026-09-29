// Package natsresources owns the fixed JetStream substrate used by the future
// multi-node runtime. It is deliberately independent of the SQL application.
package natsresources

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
)

type Names struct {
	State, Positions, Presence, SnapshotIndex, Objects string
}

var RequiredNames = Names{"FS_STATE", "FS_POSITIONS", "FS_PRESENCE", "FS_SNAPSHOT_INDEX", "FS_OBJECTS"}

type Config struct {
	URLs            []string
	CredentialsFile string
	RootCAFile      string
	ClientCertFile  string
	ClientKeyFile   string
	ConnectTimeout  time.Duration
	RequestTimeout  time.Duration
	Names           Names
}

func ConfigFromEnv() (Config, error) {
	c := Config{
		URLs:            strings.FieldsFunc(os.Getenv("NATS_URLS"), func(r rune) bool { return r == ',' }),
		CredentialsFile: strings.TrimSpace(os.Getenv("NATS_CREDENTIALS_FILE")),
		RootCAFile:      strings.TrimSpace(os.Getenv("NATS_CA_FILE")),
		ClientCertFile:  strings.TrimSpace(os.Getenv("NATS_CLIENT_CERT_FILE")),
		ClientKeyFile:   strings.TrimSpace(os.Getenv("NATS_CLIENT_KEY_FILE")),
		ConnectTimeout:  5 * time.Second,
		RequestTimeout:  5 * time.Second,
		Names:           RequiredNames,
	}
	for _, setting := range []struct {
		name   string
		target *time.Duration
	}{
		{"NATS_CONNECT_TIMEOUT", &c.ConnectTimeout}, {"NATS_REQUEST_TIMEOUT", &c.RequestTimeout},
	} {
		if value := strings.TrimSpace(os.Getenv(setting.name)); value != "" {
			duration, err := time.ParseDuration(value)
			if err != nil {
				return Config{}, fmt.Errorf("%s: %w", setting.name, err)
			}
			*setting.target = duration
		}
	}
	return c, c.Validate()
}

func (c Config) Validate() error {
	if len(c.URLs) == 0 {
		return fmt.Errorf("NATS_URLS is required")
	}
	for _, url := range c.URLs {
		if strings.TrimSpace(url) == "" {
			return fmt.Errorf("NATS_URLS contains an empty URL")
		}
	}
	if c.ConnectTimeout <= 0 || c.RequestTimeout <= 0 {
		return fmt.Errorf("NATS timeouts must be positive")
	}
	if (c.ClientCertFile == "") != (c.ClientKeyFile == "") {
		return fmt.Errorf("NATS client certificate and key must be supplied together")
	}
	if c.Names != RequiredNames {
		return fmt.Errorf("NATS resource names must match the fixed FS_* contract")
	}
	return nil
}

func Connect(c Config) (*nats.Conn, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	opts := []nats.Option{nats.Name("flightstrips-resources"), nats.Timeout(c.ConnectTimeout), nats.MaxReconnects(-1), nats.ReconnectWait(time.Second)}
	if c.CredentialsFile != "" {
		opts = append(opts, nats.UserCredentials(c.CredentialsFile))
	}
	if c.RootCAFile != "" {
		opts = append(opts, nats.RootCAs(c.RootCAFile))
	}
	if c.ClientCertFile != "" {
		opts = append(opts, nats.ClientCert(c.ClientCertFile, c.ClientKeyFile))
	}
	return nats.Connect(strings.Join(c.URLs, ","), opts...)
}
