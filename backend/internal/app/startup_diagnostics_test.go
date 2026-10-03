package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/nats-io/nats.go"
)

func TestStartupFailureDiagnosticsPreserveCauseWithoutRenderingSecrets(t *testing.T) {
	for _, test := range []struct {
		cause error
		class string
	}{
		{nats.ErrTimeout, "timeout"},
		{context.DeadlineExceeded, "timeout"},
		{nats.ErrNoResponders, "no_responders"},
		{nats.ErrStreamNotFound, "stream_not_found"},
		{nats.ErrConnectionClosed, "connection_closed"},
		{context.Canceled, "canceled"},
	} {
		err := &startupStageError{stage: "resource_verify", cause: fmt.Errorf("nats://private-user:private-password@host: %w", test.cause)}
		if !errors.Is(err, test.cause) {
			t.Fatal("startup stage must preserve the original cause")
		}
		stage, class, causeType := StartupFailureDiagnostics(err)
		if stage != "resource_verify" || class != test.class || causeType == "" {
			t.Fatalf("unexpected structural diagnostics: %s %s %s", stage, class, causeType)
		}
		if strings.Contains(err.Error()+stage+class+causeType, "private-") {
			t.Fatal("startup diagnostics leaked a credential-bearing message")
		}
	}
	api := &nats.APIError{Code: 503, ErrorCode: 10008, Description: "private-provider-payload"}
	err := &startupStageError{stage: "projection_construct", cause: api}
	var original *nats.APIError
	if !errors.As(err, &original) || original != api {
		t.Fatal("startup stage must preserve the concrete API cause")
	}
	stage, class, causeType := StartupFailureDiagnostics(err)
	if stage != "projection_construct" || class != "other" || causeType != "*nats.APIError" {
		t.Fatalf("unexpected API diagnostics: %s %s %s", stage, class, causeType)
	}
}
