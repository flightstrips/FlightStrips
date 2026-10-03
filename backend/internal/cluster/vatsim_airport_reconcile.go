package cluster

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	pb "FlightStrips/pkg/events/cluster"
)

// VatsimAirportReconciler walks one committed global generation under the
// airport owner. Each observation has a stable command ID, so takeover can
// finish a partially applied generation without replaying accepted flights.
type VatsimAirportReconciler struct {
	Worker          AmanCandidateWorker
	EvaluatePresent AmanObservationEvaluator
	EvaluateMissing AmanObservationEvaluator
}

func (r VatsimAirportReconciler) Reconcile(ctx context.Context, airport string) error {
	if r.EvaluatePresent == nil || r.EvaluateMissing == nil || len(airport) != 4 || airport != strings.ToUpper(airport) {
		return fmt.Errorf("VATSIM airport evaluators unavailable")
	}
	checkpoint, page, revision, err := r.Worker.Source.CheckpointRevisionFor(ctx, globalRef(), "vatsim", "network-data/v3")
	if err != nil {
		return err
	}
	if checkpoint == nil || page == nil || page.GetVatsim() == nil || revision == 0 || checkpoint.Sha256 == "" {
		return fmt.Errorf("committed VATSIM generation unavailable")
	}
	// CheckpointRevisionFor already verifies the entire typed page. Share this
	// detached generation across the pass instead of loading it per arrival.
	generation := &vatsimGeneration{checkpoint: checkpoint, revision: revision, page: page.GetVatsim(), flights: make(map[string]*pb.VatsimFlight)}
	board, err := r.Worker.State.Read(ctx, airport)
	if err != nil {
		return err
	}
	present := map[string]bool{}
	for _, flight := range page.GetVatsim().Flights {
		generation.flights[flight.Callsign] = flight
		if flight != nil && flight.FlightPlan != nil && strings.EqualFold(flight.FlightPlan.Destination, airport) {
			present[flight.Callsign] = true
		}
	}
	missing := map[string]bool{}
	for _, flight := range board.Flights {
		if flight != nil && flight.LatestObservation != nil && flight.LatestObservation.Provider == "vatsim" && !flight.LatestObservation.Missing && !present[flight.Callsign] {
			missing[flight.Callsign] = true
		}
	}
	var callsigns []string
	for callsign := range missing {
		callsigns = append(callsigns, callsign)
	}
	sort.Strings(callsigns)
	var failures []error
	for _, callsign := range callsigns {
		reply := r.Worker.observeMissingVatsim(ctx, airport, callsign, r.EvaluateMissing, generation)
		if reply == nil || reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
			failures = append(failures, fmt.Errorf("retract %s: %v", callsign, reply))
		}
	}
	callsigns = callsigns[:0]
	for callsign := range present {
		callsigns = append(callsigns, callsign)
	}
	sort.Strings(callsigns)
	for _, callsign := range callsigns {
		reply := r.Worker.observeVatsim(ctx, airport, callsign, r.EvaluatePresent, generation)
		if reply == nil || reply.Status != pb.CommandReply_COMMITTED || reply.GetOutcome().GetStatus() != pb.CommandOutcome_SUCCEEDED {
			failures = append(failures, fmt.Errorf("observe %s: %v", callsign, reply))
		}
	}
	return errors.Join(failures...)
}
