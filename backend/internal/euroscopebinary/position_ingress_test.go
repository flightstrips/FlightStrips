package euroscopebinary

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"FlightStrips/internal/shared"
	es "FlightStrips/pkg/events/euroscope"
	"github.com/gorilla/websocket"
)

type ingressAuthorityFunc func(int32, string, string, *es.Envelope) error

func (f ingressAuthorityFunc) ValidateEuroScopeInbound(s int32, connection, cid string, frame *es.Envelope) error {
	return f(s, connection, cid, frame)
}

func TestQueuedPositionChecksCurrentAuthorityBeforeInbound(t *testing.T) {
	d := shared.NewPositionDispatcher(2, 8, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	defer d.Close(ctx)
	blocked, release := make(chan struct{}), make(chan struct{})
	if err := d.Submit(ctx, "ABC", func(context.Context) { close(blocked); <-release }); err != nil {
		t.Fatal(err)
	}
	<-blocked
	var revoked atomic.Bool
	var validations, mutations atomic.Int32
	frame := &es.Envelope{Event: &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: "ABC"}}}
	authority := ingressAuthorityFunc(func(s int32, connection, cid string, actual *es.Envelope) error {
		validations.Add(1)
		if s != 42 || connection != "connection" || cid != "cid" || actual != frame {
			t.Error("queued validation lost its original frame identity")
		}
		if revoked.Load() {
			return errors.New("master changed while queued")
		}
		return nil
	})
	h := Handler{Inbound: func(context.Context, int32, string, string, *es.Envelope) error { mutations.Add(1); return nil }}
	result := make(chan error, 1)
	if err := d.Submit(ctx, "ABC", func(run context.Context) {
		result <- h.validatedPositionInbound(run, time.Now(), authority, 42, "connection", "cid", frame)
	}); err != nil {
		t.Fatal(err)
	}
	if validations.Load() != 0 {
		t.Fatal("position authority check ran before its keyed predecessor finished")
	}
	revoked.Store(true)
	close(release)
	select {
	case err := <-result:
		var failure socketFailure
		if !errors.As(err, &failure) || failure.code != websocket.ClosePolicyViolation {
			t.Fatalf("queued authority rejection lost policy close: %v", err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	if validations.Load() != 1 || mutations.Load() != 0 {
		t.Fatalf("revoked queued report validated %d times and mutated %d times", validations.Load(), mutations.Load())
	}
}

func TestPositionIngressValidationOrderingAndCancellation(t *testing.T) {
	var validated bool
	frame := &es.Envelope{}
	authority := ingressAuthorityFunc(func(int32, string, string, *es.Envelope) error { validated = true; return nil })
	called := false
	h := Handler{Inbound: func(context.Context, int32, string, string, *es.Envelope) error {
		if !validated {
			t.Fatal("position processed before ingress validation")
		}
		called = true
		return nil
	}}
	if err := h.validatedPositionInbound(context.Background(), time.Now(), authority, 1, "connection", "cid", frame); err != nil || !called {
		t.Fatalf("validated report not processed: %v", err)
	}
	validated, called = false, false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := h.validatedPositionInbound(ctx, time.Now(), authority, 1, "connection", "cid", frame); !errors.Is(err, context.Canceled) || validated || called {
		t.Fatalf("canceled queued report reached authority or processing: %v, %v, %v", err, validated, called)
	}
}
