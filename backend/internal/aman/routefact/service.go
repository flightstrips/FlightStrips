// Package routefact owns authenticated EuroScope direct-to facts. It resolves
// identity, tracking authority, and navigation data on the backend before one
// atomic AMAN aggregate replacement is committed.
package routefact

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/sequence"
	"FlightStrips/internal/aman/trajectory"
	"FlightStrips/internal/coordinationrequest"
	internalModels "FlightStrips/internal/models"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	maxCommitAttempts      = 3
	maximumFutureClockSkew = 30 * time.Second
)

type Repository interface {
	aman.AirportStateReader
	aman.StateCommitter
}

type StripReader interface {
	GetByCallsign(context.Context, int32, string) (*internalModels.Strip, error)
}

type GeometryReader interface {
	navdata.GeometrySnapshotReader
	Route(context.Context, navdata.RouteKey) (navdata.RouteGeometry, error)
}

// Reconciler re-runs backend-owned route projection and prediction after the
// durable fact replacement. It deliberately accepts no client geometry or
// timing input.
type Reconciler interface {
	Reconcile(context.Context)
}

type ClearanceCorrelator interface {
	ObserveClearance(context.Context, coordinationrequest.ClearanceFact) (coordinationrequest.CommitResult, error)
}

type Dependencies struct {
	Repository Repository
	Strips     StripReader
	Geometry   GeometryReader
	Publisher  sequence.FullStatePublisher
	Reconciler Reconciler
	Now        func() time.Time
	NewID      func() string
	Correlator ClearanceCorrelator
}

type Service struct{ deps Dependencies }

type Report struct {
	Session            int32
	Airport            string
	Callsign           string
	ControllerCallsign string
	DirectToFix        *string
	ObservedAt         time.Time
}

func New(deps Dependencies) (*Service, error) {
	if deps.Repository == nil || deps.Strips == nil || deps.Geometry == nil || deps.Publisher == nil || deps.Reconciler == nil {
		return nil, errors.New("AMAN route facts require repository, strips, geometry, publisher, and reconciler")
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	if deps.NewID == nil {
		deps.NewID = uuid.NewString
	}
	return &Service{deps: deps}, nil
}

func (s *Service) ReportDirectTo(ctx context.Context, session int32, airport, callsign, controllerCallsign string, directToFix *string, observedAt time.Time) error {
	report := Report{Session: session, Airport: airport, Callsign: callsign, ControllerCallsign: controllerCallsign, DirectToFix: directToFix, ObservedAt: observedAt}
	report.Airport = strings.ToUpper(strings.TrimSpace(report.Airport))
	report.Callsign = strings.ToUpper(strings.TrimSpace(report.Callsign))
	report.ControllerCallsign = strings.ToUpper(strings.TrimSpace(report.ControllerCallsign))
	if report.Session <= 0 || len(report.Airport) != 4 || report.Callsign == "" || report.ControllerCallsign == "" ||
		report.ObservedAt.IsZero() || report.ObservedAt.Location() != time.UTC {
		return domain(aman.ErrorInvalidArgument, "direct-to report is incomplete")
	}
	now := s.deps.Now().UTC()
	if report.ObservedAt.After(now.Add(maximumFutureClockSkew)) {
		return domain(aman.ErrorInvalidArgument, "direct-to observation cannot be in the future")
	}
	if report.ObservedAt.After(now) {
		report.ObservedAt = now
	}
	var fix *string
	if report.DirectToFix != nil {
		normalized := strings.ToUpper(strings.TrimSpace(*report.DirectToFix))
		if normalized != "" {
			fix = &normalized
		}
	}

	for attempt := 0; attempt < maxCommitAttempts; attempt++ {
		strip, err := s.authorize(ctx, report)
		if err != nil {
			return err
		}
		state, err := s.deps.Repository.LoadAirportState(ctx, report.Airport)
		if err != nil {
			return err
		}
		flightIndex := -1
		for index := range state.Flights {
			if matchesFlight(state.Flights[index], strip, report.Callsign) &&
				state.Flights[index].State != aman.StateLanded && state.Flights[index].State != aman.StateRemoved {
				if flightIndex >= 0 {
					return domain(aman.ErrorActiveFlightConflict, "callsign resolves to multiple active AMAN flights")
				}
				flightIndex = index
			}
		}
		if flightIndex < 0 {
			return domain(aman.ErrorNotFound, "active AMAN flight was not found")
		}

		snapshot, err := s.deps.Geometry.ActiveGeometrySnapshot(ctx, navdata.AirportID(report.Airport))
		if err != nil {
			return err
		}
		dataset := datasetVersion(snapshot.Manifest.Version)
		if fix != nil && !containsCompleteFix(snapshot, *fix) {
			return domain(aman.ErrorDegradedOrIncompleteGeometry, "direct-to fix is unknown or incomplete in active navigation data")
		}
		flight := state.Flights[flightIndex]
		if fix != nil {
			feederFix, legacySTARFamily := flight.TerminalPathIdentity()
			if flight.ActiveRouteKey == nil || (feederFix == "" && legacySTARFamily == "") || flight.SelectedRunwayGroup == nil {
				return domain(aman.ErrorDegradedOrIncompleteGeometry, "flight has no active route geometry for direct-to validation")
			}
			route, routeErr := s.deps.Geometry.Route(ctx, navdata.RouteKey(*flight.ActiveRouteKey))
			if routeErr != nil {
				return routeErr
			}
			flightPlanRevision := uint64(0)
			if flight.RouteProgress != nil {
				flightPlanRevision = flight.RouteProgress.FlightPlanRevision
			}
			if !trajectory.DirectToTargetOnForwardPath(snapshot, route, trajectory.Input{
				FeederFix: navdata.FixID(feederFix), Feeder: navdata.FeederID(legacySTARFamily), RunwayGroup: *flight.SelectedRunwayGroup,
				FlightPlanRevision: flightPlanRevision, Prior: flight.RouteProgress,
			}, navdata.FixID(*fix)) {
				return domain(aman.ErrorDegradedOrIncompleteGeometry, "direct-to fix is not on the flight's forward route")
			}
		}
		// Re-read current tracking authority after navigation resolution so a
		// controller that handed the aircraft off while this request was being
		// processed cannot commit using its earlier authority.
		if _, err := s.authorize(ctx, report); err != nil {
			return err
		}

		current := state.Flights[flightIndex].ActiveRouteFact
		if sameFact(current, fix) {
			if fix != nil && s.deps.Correlator != nil {
				return s.correlateDirect(context.WithoutCancel(ctx), report.Airport, state.Flights[flightIndex].ID, current.ID, *fix, current.Issuer, current.ObservedAt)
			}
			return nil
		}
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		factState, factFix := aman.RouteFactCleared, ""
		if fix != nil {
			factState, factFix = aman.RouteFactActive, *fix
		}
		state.Flights[flightIndex].ActiveRouteFact = &aman.RouteFact{
			ID: s.deps.NewID(), FlightID: state.Flights[flightIndex].ID, Fix: factFix, Issuer: report.ControllerCallsign,
			ObservedAt: report.ObservedAt, ReceivedAt: now, DatasetVersion: dataset, State: factState,
		}
		state.Revision++
		state.GeneratedAt = now
		committed, err := s.deps.Repository.Commit(ctx, aman.StateCommit{ExpectedRevision: state.Revision - 1, State: state})
		if err != nil {
			var domainErr *aman.DomainError
			if errors.As(err, &domainErr) && domainErr.Class == aman.ErrorRevisionConflict {
				continue
			}
			return err
		}
		publishCtx := context.WithoutCancel(ctx)
		if err := s.deps.Publisher.PublishAMANState(publishCtx, committed.State); err != nil {
			return err
		}
		// The operational engine reloads the committed aggregate, resolves the
		// direct leg from cached canonical geometry, and preserves frozen
		// operational TETA/slot policy while updating raw drift.
		s.deps.Reconciler.Reconcile(publishCtx)
		if fix != nil && s.deps.Correlator != nil {
			return s.correlateDirect(publishCtx, report.Airport, state.Flights[flightIndex].ID, state.Flights[flightIndex].ActiveRouteFact.ID, *fix, report.ControllerCallsign, report.ObservedAt)
		}
		return nil
	}
	return domain(aman.ErrorRevisionConflict, "direct-to fact conflicted with concurrent AMAN updates")
}

func (s *Service) correlateDirect(ctx context.Context, airport string, flightID aman.FlightID, factID, fix, issuer string, observedAt time.Time) error {
	_, err := s.deps.Correlator.ObserveClearance(ctx, coordinationrequest.ClearanceFact{Airport: airport,
		FlightID: coordinationrequest.FlightID(flightID), FactID: factID, Kind: coordinationrequest.KindRouteDirect,
		Value: fix, Issuer: issuer, ObservedAt: observedAt})
	if err != nil {
		return fmt.Errorf("correlate direct-to clearance: %w", err)
	}
	return nil
}

// ReportSpeed correlates a controller-assigned EuroScope speed without using
// agreement as, or introducing the value into, a prediction input.
func (s *Service) ReportSpeed(ctx context.Context, session int32, airport, callsign, controllerCallsign, value string, observedAt time.Time) error {
	report := Report{Session: session, Airport: strings.ToUpper(strings.TrimSpace(airport)), Callsign: strings.ToUpper(strings.TrimSpace(callsign)),
		ControllerCallsign: strings.ToUpper(strings.TrimSpace(controllerCallsign)), ObservedAt: observedAt}
	value = strings.ToUpper(strings.TrimSpace(value))
	now := s.deps.Now().UTC()
	if report.Session <= 0 || len(report.Airport) != 4 || report.Callsign == "" || report.ControllerCallsign == "" || value == "" || !utcObservation(observedAt, now) {
		return domain(aman.ErrorInvalidArgument, "speed report is incomplete")
	}
	if observedAt.After(now) {
		observedAt, report.ObservedAt = now, now
	}
	strip, err := s.authorize(ctx, report)
	if err != nil {
		return err
	}
	state, err := s.deps.Repository.LoadAirportState(ctx, report.Airport)
	if err != nil {
		return err
	}
	for _, flight := range state.Flights {
		if matchesFlight(flight, strip, report.Callsign) && flight.State != aman.StateLanded && flight.State != aman.StateRemoved {
			if s.deps.Correlator == nil {
				return nil
			}
			_, err = s.deps.Correlator.ObserveClearance(context.WithoutCancel(ctx), coordinationrequest.ClearanceFact{
				Airport: report.Airport, FlightID: coordinationrequest.FlightID(flight.ID), FactID: s.deps.NewID(), Kind: coordinationrequest.KindSpeed,
				Value: value, Issuer: report.ControllerCallsign, ObservedAt: observedAt,
			})
			return err
		}
	}
	return domain(aman.ErrorNotFound, "active AMAN flight was not found")
}

func utcObservation(value, now time.Time) bool {
	return !value.IsZero() && value.Location() == time.UTC && !value.After(now.Add(maximumFutureClockSkew))
}

func (s *Service) authorize(ctx context.Context, report Report) (*internalModels.Strip, error) {
	strip, err := s.deps.Strips.GetByCallsign(ctx, report.Session, report.Callsign)
	if err != nil {
		return nil, domain(aman.ErrorUnauthorized, "flight is not bound to the authenticated session")
	}
	if strip == nil || !strings.EqualFold(strings.TrimSpace(strip.TrackingController), report.ControllerCallsign) {
		return nil, domain(aman.ErrorUnauthorized, "only the current tracking controller may report a direct-to fact")
	}
	return strip, nil
}

func matchesFlight(flight aman.AMANFlight, strip *internalModels.Strip, callsign string) bool {
	if !strings.EqualFold(flight.CurrentCallsign, callsign) {
		return false
	}
	return strip.VatsimCID == nil || strings.TrimSpace(*strip.VatsimCID) == "" || strings.TrimSpace(flight.VATSIMCID) == strings.TrimSpace(*strip.VatsimCID)
}

func containsCompleteFix(snapshot navdata.ActiveGeometrySnapshot, identifier string) bool {
	for _, fix := range snapshot.Fixes {
		if strings.EqualFold(string(fix.ID), identifier) && fix.Validate() == nil {
			return true
		}
	}
	return false
}

func sameFact(current *aman.RouteFact, fix *string) bool {
	if fix == nil {
		return current == nil || current.State == aman.RouteFactCleared
	}
	return current != nil && (current.State == "" || current.State == aman.RouteFactActive) && strings.EqualFold(current.Fix, *fix)
}

func datasetVersion(version navdata.DatasetVersion) string {
	return strings.Join([]string{
		version.Cycle,
		version.SourceRevision,
		version.EffectiveFrom.UTC().Format(time.RFC3339Nano),
		version.EffectiveUntil.UTC().Format(time.RFC3339Nano),
	}, "|")
}

func domain(class aman.ErrorClass, message string) error {
	return &aman.DomainError{Class: class, Message: fmt.Sprintf("AMAN route fact: %s", message)}
}
