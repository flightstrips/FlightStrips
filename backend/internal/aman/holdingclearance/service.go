// Package holdingclearance owns durable replacement of authoritative strip
// holding facts on the AMAN flight aggregate.
package holdingclearance

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
	"context"
	"errors"
	"slices"
	"strings"
	"time"
)

const maxCommitAttempts = 3

type Repository interface {
	aman.AirportStateReader
	aman.StateCommitter
}

type Dependencies struct {
	Repository Repository
	Publisher  sequence.FullStatePublisher
}

type Service struct{ deps Dependencies }

func New(deps Dependencies) (*Service, error) {
	if deps.Repository == nil || deps.Publisher == nil {
		return nil, errors.New("AMAN holding clearances require repository and publisher")
	}
	return &Service{deps: deps}, nil
}

func (s *Service) ObserveHoldingClearance(ctx context.Context, fact aman.HoldingClearanceFact) error {
	return s.ObserveHoldingClearances(ctx, []aman.HoldingClearanceFact{fact})
}

// ObserveHoldingClearances applies a sync's facts with one aggregate revision and
// publication per airport. A conflict reloads and reapplies the entire batch.
func (s *Service) ObserveHoldingClearances(ctx context.Context, facts []aman.HoldingClearanceFact) error {
	byAirport := make(map[string][]aman.HoldingClearanceFact)
	var airports []string
	for _, fact := range facts {
		fact.Destination = strings.ToUpper(strings.TrimSpace(fact.Destination))
		if fact.FlightID == "" || fact.Destination == "" || fact.ObservedAt.IsZero() || fact.ObservedAt.Location() != time.UTC {
			return &aman.DomainError{Class: aman.ErrorInvalidArgument, Message: "AMAN holding clearance fact is incomplete"}
		}
		if _, exists := byAirport[fact.Destination]; !exists {
			airports = append(airports, fact.Destination)
		}
		byAirport[fact.Destination] = append(byAirport[fact.Destination], fact)
	}
	slices.Sort(airports)
	for _, airport := range airports {
		if err := s.observeAirportClearances(ctx, airport, byAirport[airport]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) observeAirportClearances(ctx context.Context, airport string, facts []aman.HoldingClearanceFact) error {
	for attempt := 0; attempt < maxCommitAttempts; attempt++ {
		state, err := s.deps.Repository.LoadAirportState(ctx, airport)
		if err != nil {
			var domainErr *aman.DomainError
			if errors.As(err, &domainErr) && domainErr.Class == aman.ErrorNotFound {
				return nil
			}
			return err
		}
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		indices := make(map[aman.FlightID]int, len(state.Flights))
		for index, flight := range state.Flights {
			indices[flight.ID] = index
		}
		changed := false
		for _, fact := range facts {
			index, exists := indices[fact.FlightID]
			if !exists || state.Flights[index].VATSIMCID != strings.TrimSpace(fact.VATSIMCID) {
				continue
			}
			normalized := normalize(fact)
			current := state.Flights[index].HoldingClearance
			if sameClearance(current, normalized) || (current != nil && !fact.ObservedAt.After(current.ObservedAt)) {
				continue
			}
			state.Flights[index].HoldingClearance = normalized
			changed = true
			if fact.ObservedAt.After(state.GeneratedAt) {
				state.GeneratedAt = fact.ObservedAt
			}
		}
		if !changed {
			return nil
		}
		expectedRevision := state.Revision
		advanceRevision(&state)
		committed, err := s.deps.Repository.Commit(ctx, aman.StateCommit{ExpectedRevision: expectedRevision, State: state})
		if err == nil {
			return s.deps.Publisher.PublishAMANState(context.WithoutCancel(ctx), committed.State)
		}
		var domainErr *aman.DomainError
		if !errors.As(err, &domainErr) || domainErr.Class != aman.ErrorRevisionConflict {
			return err
		}
	}
	return &aman.DomainError{Class: aman.ErrorRevisionConflict, Message: "AMAN holding clearance conflicted with concurrent updates"}
}

func advanceRevision(state *aman.AirportState) {
	state.Revision++
	for index := range state.Flights {
		flight := &state.Flights[index]
		if flight.Slot != nil {
			flight.Slot.Revision = state.Revision
		}
		for offerIndex := range flight.QueueOffers {
			flight.QueueOffers[offerIndex].AirportRevision = state.Revision
			flight.QueueOffers[offerIndex].CandidateSlot.Revision = state.Revision
		}
	}
}

func normalize(fact aman.HoldingClearanceFact) *aman.HoldingClearance {
	hold := strings.ToUpper(strings.TrimSpace(fact.Hold))
	holdType := aman.HoldingClearanceType(strings.ToLower(strings.TrimSpace(string(fact.HoldType))))
	holdEAT := strings.TrimSpace(fact.HoldEAT)
	altitude := cloneAltitude(fact.ClearedAltitude)
	if hold == "" {
		holdType = ""
		holdEAT = ""
		altitude = nil
	}
	return &aman.HoldingClearance{
		Hold: hold, HoldType: holdType, HoldEAT: holdEAT,
		ClearedAltitude: altitude, ObservedAt: fact.ObservedAt,
	}
}

func sameClearance(current, next *aman.HoldingClearance) bool {
	if current == nil || next == nil {
		return current == next
	}
	return current.Hold == next.Hold && current.HoldType == next.HoldType && current.HoldEAT == next.HoldEAT &&
		equalAltitude(current.ClearedAltitude, next.ClearedAltitude)
}

func equalAltitude(left, right *int32) bool {
	return left == nil && right == nil || left != nil && right != nil && *left == *right
}

func cloneAltitude(value *int32) *int32 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
