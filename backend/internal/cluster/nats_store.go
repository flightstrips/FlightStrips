package cluster

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/nats-io/nats.go"
)

// NATSStore uses the existing, explicitly bootstrapped FS_STATE stream. It
// creates no resources and makes no connection in the current SQL runtime.
type NATSStore struct{ JS nats.JetStreamContext }

func (s NATSStore) Replay(ctx context.Context, subject string) ([]AppliedEvent, error) {
	if s.JS == nil {
		return nil, fmt.Errorf("missing JetStream context")
	}
	info, err := s.JS.StreamInfo("FS_STATE", &nats.StreamInfoRequest{SubjectsFilter: subject}, nats.Context(ctx))
	if err != nil {
		return nil, err
	}
	count := info.State.Subjects[subject]
	entries := make([]AppliedEvent, 0, count)
	if count == 0 {
		return entries, nil
	}
	sub, err := s.JS.SubscribeSync(subject, nats.BindStream("FS_STATE"), nats.DeliverAll(), nats.OrderedConsumer())
	if err != nil {
		return nil, err
	}
	defer sub.Unsubscribe()
	for i := uint64(0); i < count; i++ {
		message, err := sub.NextMsgWithContext(ctx)
		if err != nil {
			return nil, err
		}
		metadata, err := message.Metadata()
		if err != nil {
			return nil, err
		}
		// Nats-Expected-Last-Subject-Sequence expects the last global stream
		// sequence seen on this subject, not a per-subject message count.
		entries = append(entries, AppliedEvent{Subject: message.Subject, StreamSequence: metadata.Sequence.Stream, SubjectSequence: metadata.Sequence.Stream, ServerTime: metadata.Timestamp, Data: message.Data})
	}
	return entries, nil
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
	ack, err := s.JS.PublishMsg(message, nats.Context(ctx))
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
	return ack.Sequence, nil
}
