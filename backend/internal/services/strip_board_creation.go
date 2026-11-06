package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/vatsim"
	pkg "FlightStrips/pkg/models"
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

// SetConnectedFlightSource binds the application's existing snapshot source at startup.
// It creates no client, polling loop, session cache or mutable current airport.
func (s *StripService) SetConnectedFlightSource(source vatsim.SnapshotSource, maxAge time.Duration, now func() time.Time) {
	s.connectedSource = source
	s.connectedSourceMaxAge = maxAge
	s.boardNow = now
}
func (s *StripService) boardTime() time.Time {
	if s.boardNow != nil {
		return s.boardNow().UTC()
	}
	return time.Now().UTC()
}

func (s *StripService) authorizeStripBoardContext(ctx context.Context, session int32, cid, airport, revision string) (*airportworkflow.Bound, error) {
	if _, ok := shared.HumanCommand(ctx); !ok {
		return nil, shared.RejectAction("exact own plugin binding required")
	}
	if _, err := shared.EnsureHumanCommand(ctx, s.esCommander, session, cid); err != nil {
		return nil, err
	}
	b, err := s.workflowFor(ctx, session, airport)
	if err != nil {
		return nil, err
	}
	if !b.BoardStatePolicy().Revisioned || b.Definition().Revision() != revision {
		return nil, shared.RejectAction("stale or unavailable strip board")
	}
	return b, nil
}

type stripBoardAdmissionStore interface {
	AdmitStripBoardEntry(context.Context, *models.Strip, bool, func() error) error
}

func (s *StripService) CreateLocalStrip(ctx context.Context, session int32, cid, airport string, req pkg.LocalStripCreation) error {
	b, err := s.authorizeStripBoardContext(ctx, session, cid, airport, req.Definition)
	if err != nil {
		return err
	}
	req.Callsign = strings.ToUpper(strings.TrimSpace(req.Callsign))
	req.Origin = strings.ToUpper(strings.TrimSpace(req.Origin))
	req.Destination = strings.ToUpper(strings.TrimSpace(req.Destination))
	if req.Origin == "" {
		req.Origin = airport
	}
	if req.Destination == "" {
		req.Destination = airport
	}
	if !regexp.MustCompile(`^[A-Z0-9]{2,10}$`).MatchString(req.Callsign) || !regexp.MustCompile(`^[A-Z]{4}$`).MatchString(req.Origin) || !regexp.MustCompile(`^[A-Z]{4}$`).MatchString(req.Destination) || req.Origin != airport && req.Destination != airport {
		return shared.RejectAction("invalid airport-local strip identity")
	}
	if len(req.Aircraft) > 16 || len(req.Route) > 2048 || len(req.Notes) > 2048 || len(req.Registration) > 16 || req.RequestedAltitude < 0 || req.RequestedAltitude > 60000 {
		return shared.RejectAction("invalid local strip fields")
	}
	if req.Runway != "" && !slices.Contains(b.Definition().Document().Geography.Runways, req.Runway) {
		return shared.RejectAction("unknown airport runway")
	}
	decision, err := b.Action(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource, Now: s.boardTime()}, "strip.create-local", req)
	if err != nil {
		return err
	}
	if decision.Admission == nil {
		return shared.RejectAction("workflow omitted local admission")
	}
	bay := decision.Admission.Bay
	created := s.boardTime()
	state := &models.StripBoardState{CreatedAt: &created, Local: true, Annotations: decision.Admission.Annotations}
	strip := &models.Strip{Callsign: req.Callsign, Session: session, Origin: req.Origin, Destination: req.Destination, AircraftType: &req.Aircraft, Route: &req.Route, Registration: &req.Registration, RequestedAltitude: &req.RequestedAltitude, Runway: &req.Runway, Bay: bay, IsManual: true, HasFP: false, BoardState: state}
	return s.admitStripBoardEntry(ctx, strip, false, nil)
}

func (s *StripService) connectedSnapshot(ctx context.Context, session int32) (vatsim.Snapshot, error) {
	model, err := s.getSessionRepository().GetByID(ctx, session)
	if err != nil {
		return vatsim.Snapshot{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(model.Name), "LIVE") {
		return vatsim.Snapshot{}, shared.RejectAction("live connected-pilot lookup is unavailable in this environment")
	}
	return s.currentConnectedSnapshot()
}

// Snapshot acquisition is an in-memory read of the shared source; it is also
// safe to revalidate after waiting for an admission transaction lock.
func (s *StripService) currentConnectedSnapshot() (vatsim.Snapshot, error) {
	if s.connectedSource == nil || s.connectedSourceMaxAge <= 0 {
		return vatsim.Snapshot{}, shared.RejectAction("shared connected-pilot source unavailable")
	}
	snapshot := s.connectedSource.Snapshot()
	if snapshot.Timestamp.IsZero() || snapshot.LastRefreshError != nil || s.boardTime().Before(snapshot.Timestamp) || s.boardTime().Sub(snapshot.Timestamp) >= s.connectedSourceMaxAge {
		return vatsim.Snapshot{}, shared.RejectAction("shared connected-pilot source is stale")
	}
	return snapshot, nil
}
func airportConnectedFlight(f vatsim.Flight, airport string) bool {
	return f.Online() && (f.FlightPlan.Origin == airport || f.FlightPlan.Destination == airport)
}
func connectedFlightCandidate(f vatsim.Flight) pkg.ConnectedFlightCandidate {
	return pkg.ConnectedFlightCandidate{Callsign: f.Callsign, CID: f.CID, LogonTime: f.LogonTime, SourceRevision: f.FlightPlan.Revision, Origin: f.FlightPlan.Origin, Destination: f.FlightPlan.Destination, Aircraft: f.FlightPlan.AircraftShort}
}

func (s *StripService) FindConnectedFlights(ctx context.Context, session int32, cid, airport, revision, query string) ([]pkg.ConnectedFlightCandidate, error) {
	b, err := s.authorizeStripBoardContext(ctx, session, cid, airport, revision)
	if err != nil {
		return nil, err
	}
	decision, err := b.Action(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource, Now: s.boardTime()}, "flight.find-connected", airportworkflow.ConnectedFlightSearch{Query: query})
	if err != nil {
		return nil, err
	}
	if decision.ConnectedFlights == nil {
		return nil, shared.RejectAction("workflow omitted connected lookup")
	}
	query = decision.ConnectedFlights.Query
	if len(query) > 64 {
		return nil, shared.RejectAction("lookup text too long")
	}
	snapshot, err := s.connectedSnapshot(ctx, session)
	if err != nil {
		return nil, err
	}
	query = strings.ToUpper(strings.TrimSpace(query))
	result := []pkg.ConnectedFlightCandidate{}
	for _, flight := range snapshot.Flights() {
		if airportConnectedFlight(flight, airport) && strings.Contains(flight.Callsign, query) {
			result = append(result, connectedFlightCandidate(flight))
		}
	}
	if err = shared.ValidateCommandAuthority(ctx, session); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *StripService) AdmitConnectedFlight(ctx context.Context, session int32, cid, airport, revision string, selection pkg.ConnectedFlightCandidate) error {
	b, err := s.authorizeStripBoardContext(ctx, session, cid, airport, revision)
	if err != nil {
		return err
	}
	snapshot, err := s.connectedSnapshot(ctx, session)
	if err != nil {
		return err
	}
	f, ok := snapshot.FlightByCallsign(selection.Callsign)
	if !ok || !airportConnectedFlight(f, airport) || f.CID != selection.CID || !f.LogonTime.Equal(selection.LogonTime) || f.FlightPlan.Revision != selection.SourceRevision {
		return shared.RejectAction("connected flight changed; repeat FIND")
	}
	decision, err := b.Action(airportworkflow.Context{Session: session, Source: airportworkflow.HumanSource, Now: s.boardTime()}, "strip.admit-connected", connectedFlightCandidate(f))
	if err != nil {
		return err
	}
	if decision.Admission == nil {
		return shared.RejectAction("workflow omitted connected admission")
	}
	bay := decision.Admission.Bay
	aircraft := f.FlightPlan.AircraftShort
	if aircraft == "" {
		aircraft = f.FlightPlan.Aircraft
	}
	created := s.boardTime()
	strip := &models.Strip{Callsign: f.Callsign, Session: session, Origin: f.FlightPlan.Origin, Destination: f.FlightPlan.Destination, AircraftType: &aircraft, Alternative: &f.FlightPlan.Alternate, FlightRules: f.FlightPlan.FlightRules, Route: &f.FlightPlan.Route, Remarks: &f.FlightPlan.Remarks, AssignedSquawk: &f.FlightPlan.AssignedSquawk, Bay: bay, HasFP: false, VatsimCID: &f.CID, VatsimRevision: &f.FlightPlan.Revision, VatsimSeenAt: &snapshot.Timestamp, BoardState: &models.StripBoardState{CreatedAt: &created, Annotations: decision.Admission.Annotations}}
	return s.admitStripBoardEntry(ctx, strip, true, func() error {
		current, err := s.currentConnectedSnapshot()
		if err != nil {
			return err
		}
		flight, ok := current.FlightByCallsign(selection.Callsign)
		if !ok || !airportConnectedFlight(flight, airport) || flight.CID != selection.CID || !flight.LogonTime.Equal(selection.LogonTime) || flight.FlightPlan.Revision != selection.SourceRevision {
			return shared.RejectAction("connected flight changed; repeat FIND")
		}
		return nil
	})
}
func (s *StripService) admitStripBoardEntry(ctx context.Context, strip *models.Strip, existing bool, sourceGuard func() error) error {
	store, ok := s.stripReader.(stripBoardAdmissionStore)
	if !ok {
		return fmt.Errorf("strip board admission persistence unavailable")
	}
	if err := shared.ValidateCommandAuthority(ctx, strip.Session); err != nil {
		return err
	}
	guard := func() error {
		if err := shared.ValidateCommandAuthority(ctx, strip.Session); err != nil {
			return err
		}
		if sourceGuard != nil {
			return sourceGuard()
		}
		return nil
	}
	if err := store.AdmitStripBoardEntry(ctx, strip, existing, guard); err != nil {
		return err
	}
	shared.PublishStripUpdate(ctx, s.publisher, strip.Session, strip.Callsign)
	return nil
}
