package cluster

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"FlightStrips/internal/faultgate"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

// NATSStore uses the existing, explicitly bootstrapped FS_STATE stream. It
// creates no resources and makes no connection in the current SQL runtime.
type NATSStore struct{ JS nats.JetStreamContext }

// Committed returns broker metadata for one acknowledged event. In particular,
// lease validity is decided by the server timestamp, never the owner's clock.
func (s NATSStore) Committed(ctx context.Context, sequence uint64) (AppliedEvent, error) {
	msg, err := s.JS.GetMsg("FS_STATE", sequence, nats.Context(ctx))
	if err != nil {
		return AppliedEvent{}, err
	}
	if msg == nil || msg.Sequence != sequence || msg.Time.IsZero() {
		return AppliedEvent{}, fmt.Errorf("invalid committed event metadata")
	}
	return AppliedEvent{Subject: msg.Subject, StreamSequence: sequence, SubjectSequence: sequence, ServerTime: msg.Time, Data: msg.Data}, nil
}

func (s NATSStore) Replay(ctx context.Context, subject string) ([]AppliedEvent, error) {
	entries := []AppliedEvent{}
	err := s.Visit(ctx, subject, func(entry AppliedEvent) error { entries = append(entries, entry); return nil })
	return entries, err
}

// Visit consumes history without accumulating an unbounded slice of events.
func (s NATSStore) Visit(ctx context.Context, subject string, visit func(AppliedEvent) error) error {
	if s.JS == nil {
		return fmt.Errorf("missing JetStream context")
	}
	info, err := s.JS.StreamInfo("FS_STATE", &nats.StreamInfoRequest{SubjectsFilter: subject}, nats.Context(ctx))
	if err != nil {
		return err
	}
	count := info.State.Subjects[subject]
	if count == 0 {
		return nil
	}
	sub, err := s.JS.SubscribeSync(subject, nats.BindStream("FS_STATE"), nats.DeliverAll(), nats.OrderedConsumer())
	if err != nil {
		return err
	}
	defer sub.Unsubscribe()
	for i := uint64(0); i < count; i++ {
		message, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			return err
		}
		metadata, err := message.Metadata()
		if err != nil {
			return err
		}
		// Nats-Expected-Last-Subject-Sequence expects the last global stream
		// sequence seen on this subject, not a per-subject message count.
		if err := visit(AppliedEvent{Subject: message.Subject, StreamSequence: metadata.Sequence.Stream, SubjectSequence: metadata.Sequence.Stream, ServerTime: metadata.Timestamp, Data: message.Data}); err != nil {
			return err
		}
	}
	return nil
}

func (s NATSStore) Publish(ctx context.Context, subject string, expected uint64, data []byte) (uint64, error) {
	if s.JS == nil {
		return 0, fmt.Errorf("missing JetStream context")
	}
	if len(data) > MaxStateBytes {
		return 0, fmt.Errorf("oversized state event")
	}
	message := nats.NewMsg(subject)
	message.Header.Set(nats.ExpectedLastSubjSeqHdr, strconv.FormatUint(expected, 10))
	message.Data = data
	faultgate.State("before-publish", data, 0)
	_, span := otel.Tracer("cluster").Start(ctx, "nats.state.puback")
	ack, err := s.JS.PublishMsg(message, nats.Context(ctx))
	if err != nil {
		span.SetStatus(codes.Error, "publish failed")
	}
	span.End()
	if err != nil {
		var api *nats.APIError
		// nats.go names 10071; NATS 2.15 returns 10164 for the
		// subject-specific expectation. Both mean a conditional write lost.
		if errors.As(err, &api) && (api.ErrorCode == nats.JSErrCodeStreamWrongLastSequence || api.ErrorCode == 10164) {
			return 0, ErrCAS
		}
		return 0, err
	}
	if ack == nil || ack.Stream != "FS_STATE" || ack.Sequence == 0 {
		return 0, fmt.Errorf("invalid PubAck")
	}
	faultgate.State("after-puback", data, ack.Sequence)
	return ack.Sequence, nil
}
