package natsresources

import (
	"strings"
	"testing"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("NATS_URLS", "nats://one:4222,nats://two:4222")
	t.Setenv("NATS_REQUEST_TIMEOUT", "bogus")
	if _, err := ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "NATS_REQUEST_TIMEOUT") {
		t.Fatalf("expected timeout error, got %v", err)
	}
	t.Setenv("NATS_REQUEST_TIMEOUT", "3s")
	t.Setenv("NATS_CLIENT_CERT_FILE", "cert.pem")
	if _, err := ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "certificate and key") {
		t.Fatalf("expected TLS pair error, got %v", err)
	}
	t.Setenv("NATS_CLIENT_KEY_FILE", "key.pem")
	c, err := ConfigFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if len(c.URLs) != 2 || c.RequestTimeout.Seconds() != 3 || c.Names != RequiredNames {
		t.Fatalf("unexpected NATS config: %+v", c)
	}
}
