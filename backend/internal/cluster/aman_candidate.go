package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

// AmanCandidateWorker consumes a committed global VATSIM generation on the
// airport owner. Its evaluator is the injected operational AMAN policy; the
// worker owns source identity, command identity, commit and intent recovery.
type AmanCandidateWorker struct {
	Source  NavigationWeather
	State   AmanAdapter
	Intents AmanIntentRunner
}

type AmanObservationEvaluator func(context.Context, AmanBoard, *pb.VatsimFlight, *pb.VatsimObservation, *pb.CommandRequest) (AmanTransition, error)
type AmanReconciliationEvaluator func(context.Context, AmanBoard, time.Time) (AmanTransition, error)

func (w AmanCandidateWorker) ObserveVatsim(ctx context.Context, icao, callsign string, evaluate AmanObservationEvaluator) *pb.CommandReply {
	if evaluate == nil || icao == "" || icao != strings.ToUpper(icao) || callsign == "" {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
	}
	checkpoint, page, revision, err := w.Source.CheckpointRevisionFor(ctx, globalRef(), "vatsim", "network-data/v3")
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
	}
	if checkpoint == nil || page == nil || page.GetVatsim() == nil || revision == 0 || checkpoint.Sha256 == "" {
		return &pb.CommandReply{Status: pb.CommandReply_NOT_FOUND, Detail: "committed VATSIM generation unavailable"}
	}
	if err := validateProviderPage(page); err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	var flight *pb.VatsimFlight
	for _, item := range page.GetVatsim().Flights {
		if item.Callsign == strings.ToUpper(callsign) {
			flight = item
			break
		}
	}
	if flight == nil || flight.FlightPlan == nil || !strings.EqualFold(flight.FlightPlan.Destination, icao) {
		return &pb.CommandReply{Status: pb.CommandReply_NOT_FOUND, Detail: "arrival absent from VATSIM generation"}
	}
	board, err := w.State.Read(ctx, icao)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
	}
	prior := uint64(0)
	if board.Airport != nil {
		prior = board.Airport.Revision
	}
	observationID := fmt.Sprintf("%s/%020d/%s/%s", icao, revision, flight.Callsign, checkpoint.Sha256)
	observation := &pb.VatsimObservation{ProviderId: observationID, Callsign: flight.Callsign, Digest: checkpoint.Sha256, ObservedAt: page.GetVatsim().SnapshotAt}
	commandID, err := AmanObservationCommandID("vatsim", observationID)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	if prior := w.State.Writer.Outcome(ctx, airportRef(icao), commandID, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "vatsim-adapter"}); prior.Status != pb.CommandReply_NOT_FOUND {
		return prior
	}
	request, err := AmanVatsimObservationRequest(icao, observation, prior)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	transition, err := evaluate(ctx, board, proto.Clone(flight).(*pb.VatsimFlight), proto.Clone(observation).(*pb.VatsimObservation), request)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	transition.Request = request
	transition.Observations = append(transition.Observations, observation)
	return w.State.Commit(ctx, transition)
}

// ObserveMissingVatsim records that a previously known airport arrival is
// absent from a committed global generation. The evaluator receives a nil
// flight and must retain or remove operational state according to AMAN policy.
func (w AmanCandidateWorker) ObserveMissingVatsim(ctx context.Context, icao, callsign string, evaluate AmanObservationEvaluator) *pb.CommandReply {
	if evaluate == nil || icao == "" || icao != strings.ToUpper(icao) || callsign == "" || callsign != strings.ToUpper(callsign) {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
	}
	checkpoint, page, revision, err := w.Source.CheckpointRevisionFor(ctx, globalRef(), "vatsim", "network-data/v3")
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
	}
	if checkpoint == nil || page == nil || page.GetVatsim() == nil || revision == 0 || checkpoint.Sha256 == "" {
		return &pb.CommandReply{Status: pb.CommandReply_NOT_FOUND, Detail: "committed VATSIM generation unavailable"}
	}
	if err := validateProviderPage(page); err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	for _, flight := range page.GetVatsim().Flights {
		if flight != nil && flight.Callsign == callsign && flight.FlightPlan != nil && strings.EqualFold(flight.FlightPlan.Destination, icao) {
			return &pb.CommandReply{Status: pb.CommandReply_REVISION_CONFLICT, Detail: "arrival remains in VATSIM generation"}
		}
	}
	board, err := w.State.Read(ctx, icao)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
	}
	observationID := fmt.Sprintf("%s/%020d/%s/%s/missing", icao, revision, callsign, checkpoint.Sha256)
	observation := &pb.VatsimObservation{ProviderId: observationID, Callsign: callsign, Digest: checkpoint.Sha256, ObservedAt: page.GetVatsim().SnapshotAt}
	commandID, err := AmanObservationCommandID("vatsim", observationID)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	if outcome := w.State.Writer.Outcome(ctx, airportRef(icao), commandID, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "vatsim-adapter"}); outcome.Status != pb.CommandReply_NOT_FOUND {
		return outcome
	}
	known := false
	for _, flight := range board.Flights {
		if flight.Callsign == callsign && flight.LatestObservation != nil && flight.LatestObservation.Provider == "vatsim" && !flight.LatestObservation.Missing {
			known = true
			break
		}
	}
	if !known {
		return &pb.CommandReply{Status: pb.CommandReply_NOT_FOUND, Detail: "no active VATSIM arrival to retract"}
	}
	prior := uint64(0)
	if board.Airport != nil {
		prior = board.Airport.Revision
	}
	request, err := AmanVatsimObservationRequest(icao, observation, prior)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	transition, err := evaluate(ctx, board, nil, proto.Clone(observation).(*pb.VatsimObservation), request)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	transition.Request = request
	transition.Observations = append(transition.Observations, observation)
	return w.State.Commit(ctx, transition)
}

// Reconcile evaluates one deadline from the latest airport board. The stable
// tick ID and expected airport revision make owner retries idempotent.
func (w AmanCandidateWorker) Reconcile(ctx context.Context, icao string, deadline time.Time, evaluate AmanReconciliationEvaluator) *pb.CommandReply {
	if evaluate == nil || deadline.IsZero() || icao == "" || icao != strings.ToUpper(icao) {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT}
	}
	id, err := ProviderEventCommandID("aman-reconcile", "aman", icao+"/"+deadline.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	if prior := w.State.Writer.Outcome(ctx, airportRef(icao), id, &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "aman-reconcile"}); prior.Status != pb.CommandReply_NOT_FOUND {
		return prior
	}
	board, err := w.State.Read(ctx, icao)
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_UNAVAILABLE, Detail: err.Error()}
	}
	prior := uint64(0)
	if board.Airport != nil {
		prior = board.Airport.Revision
	}
	transition, err := evaluate(ctx, board, deadline.UTC())
	if err != nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: err.Error()}
	}
	if transition.Airport == nil {
		return &pb.CommandReply{Status: pb.CommandReply_INVALID_ARGUMENT, Detail: "missing evaluated airport board"}
	}
	request := &pb.CommandRequest{ProtocolRevision: 1, CommandId: id, Aggregate: airportRef(icao), Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "aman-reconcile"}, ExpectedEntityRevision: &prior,
		Command: &pb.CommandRequest_System{System: &pb.SystemCommand{Action: &pb.SystemCommand_UpdateEntity{UpdateEntity: &pb.UpdateEntity{Key: icao, Value: &pb.EntityRecord{Value: &pb.EntityRecord_AmanAirport{AmanAirport: transition.Airport}}}}}}}
	transition.Request = request
	return w.State.Commit(ctx, transition)
}

func (w AmanCandidateWorker) Resume(ctx context.Context, icao string) error {
	return w.Intents.Resume(ctx, icao)
}
