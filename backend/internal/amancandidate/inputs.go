package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/predictor"
	"FlightStrips/internal/cluster"
	"FlightStrips/internal/vatsim"
	pb "FlightStrips/pkg/events/cluster"
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

type acceptedInputs struct {
	observations []aman.FlightObservation
	sessions     []*cluster.Aggregate
	vatsimAt     time.Time
	vatsimStatus aman.DataStatus
}

func (i *acceptedInputs) ActiveArrivalRunway(context.Context, string) (string, error) {
	active := map[string]bool{}
	for _, s := range i.sessions {
		for _, e := range s.EntitiesByKind(pb.EntityKind_SESSION) {
			for _, r := range e.GetValue().GetSession().Runways {
				if r.Arrival {
					active[r.Name] = true
				}
			}
		}
	}
	if len(active) != 1 {
		return "", fmt.Errorf("expected one accepted arrival runway, found %d", len(active))
	}
	for r := range active {
		return r, nil
	}
	return "", fmt.Errorf("arrival runway unavailable")
}
func (w *Worker) acceptedInputs(ctx context.Context, airport string, board cluster.AmanBoard, at time.Time, identity *pb.VatsimObservation) (*acceptedInputs, error) {
	result := &acceptedInputs{vatsimStatus: aman.DataDisconnected}
	previous := map[string]map[aman.ObservationProvider]aman.FlightObservation{}
	for _, f := range board.Flights {
		sources := f.SourceObservations
		if len(sources) == 0 && f.LatestObservation != nil {
			sources = []*pb.AmanFlightObservation{f.LatestObservation}
		}
		previous[f.Callsign] = map[aman.ObservationProvider]aman.FlightObservation{}
		for _, o := range sources {
			value := *decodeAmanFlightObservation(o)
			previous[f.Callsign][value.Provider] = value
		}
	}
	current := map[string]map[aman.ObservationProvider]aman.FlightObservation{}
	put := func(o aman.FlightObservation) {
		if current[o.Callsign] == nil {
			current[o.Callsign] = map[aman.ObservationProvider]aman.FlightObservation{}
		}
		prior, ok := current[o.Callsign][o.Provider]
		if !ok || o.ReconciledAt.After(prior.ReconciledAt) {
			current[o.Callsign][o.Provider] = o
		}
	}
	if w.options.SourceMode.UsesVATSIM() {
		checkpoint, page, _, err := w.options.Source.CheckpointRevisionFor(ctx, globalRef(), "vatsim", "network-data/v3")
		if err != nil {
			return nil, err
		}
		if identity != nil && (checkpoint == nil || checkpoint.Sha256 != identity.Digest || page == nil || !page.GetVatsim().SnapshotAt.AsTime().Equal(identity.ObservedAt.AsTime())) {
			return nil, fmt.Errorf("VATSIM generation changed during evaluation")
		}
		if page != nil {
			snapshot, err := vatsim.SnapshotFromPage(page)
			if err != nil {
				return nil, err
			}
			result.vatsimAt = snapshot.Timestamp
			result.vatsimStatus = aman.DataFresh
			if at.Sub(snapshot.Timestamp) > w.options.VatsimStaleAfter || at.Before(snapshot.Timestamp) {
				result.vatsimStatus = aman.DataStale
			}
			for _, typed := range page.GetVatsim().Flights {
				if !strings.EqualFold(typed.FlightPlan.Destination, airport) {
					continue
				}
				flight, found := snapshot.FlightByCallsign(typed.Callsign)
				if !found {
					return nil, fmt.Errorf("VATSIM lookup unavailable")
				}
				var prior *aman.FlightObservation
				if p, ok := previous[typed.Callsign][aman.ObservationProviderVATSIM]; ok {
					copy := p
					prior = &copy
				}
				o, err := vatsim.MapAMANObservation(flight, snapshot.Timestamp, result.vatsimStatus, snapshot.Timestamp, prior)
				if err != nil {
					return nil, err
				}
				put(o)
			}
		}
	}
	global, err := w.options.Sessions.Read(ctx, globalRef())
	if err != nil {
		return nil, err
	}
	for _, e := range global.EntitiesByKind(pb.EntityKind_SESSION_REGISTRY) {
		entry := e.GetValue().GetSessionRegistry()
		if entry.Airport != airport || entry.State != pb.SessionRegistry_ACTIVE {
			continue
		}
		s, err := w.options.Sessions.Read(ctx, sessionRef(entry.Id))
		if err != nil {
			return nil, err
		}
		result.sessions = append(result.sessions, s)
		if !w.options.SourceMode.UsesEuroScope() {
			continue
		}
		sync := s.Sync
		var positions []cluster.KVPosition
		if w.options.Projection != nil {
			sync, err = w.options.Projection.OperationalSync(sessionRef(entry.Id))
			if err != nil {
				return nil, err
			}
			positions, _, err = w.options.Projection.ObservationSnapshot(entry.Id)
			if err != nil {
				return nil, err
			}
		}
		if sync == nil {
			continue
		}
		positionByCallsign := map[string]cluster.KVPosition{}
		for _, p := range positions {
			if !p.Stale && p.Value.GetPosition() != nil {
				positionByCallsign[p.Value.AircraftKey] = p
			}
		}
		for _, e := range s.EntitiesByKind(pb.EntityKind_STRIP) {
			strip := e.GetValue().GetStrip()
			if strip.VatsimOnly || strip.Destination != airport || strip.Departure == "" {
				continue
			}
			observed := instant(sync.CompletedAt)
			if strip.EuroscopeObservedAt != nil {
				observed = instant(strip.EuroscopeObservedAt)
			}
			planRevision := uint64(max(strip.VatsimPlanRevision, 0))
			o := aman.FlightObservation{Callsign: strip.Callsign, Origin: strip.Departure, Destination: strip.Destination, AircraftType: optionalText(strip.AircraftType), WakeCategory: optionalText(strip.AircraftCategory), FiledRoute: optionalText(strip.Route), RequestedLevel: optionalNumber[int](strip.RequestedAltitude),
				FlightPlan: aman.FlightPlanFact{Revision: &planRevision, ObservedAt: &observed}, HoldingClearance: &aman.HoldingClearance{Hold: strip.Hold, HoldType: aman.HoldingClearanceType(strip.HoldType), HoldEAT: strip.HoldEat, ClearedAltitude: strip.ClearedAltitude, ObservedAt: observed}, Provider: aman.ObservationProviderEuroScope, ReconciledAt: observed, SourceStatus: aman.DataFresh}
			if p, ok := positionByCallsign[strip.Callsign]; ok {
				a := p.Value.GetPosition()
				observed = p.Value.ObservedAt.AsTime()
				alt := int(a.AltitudeFeet)
				speed, track := a.GroundSpeedKnots, a.TrackDegrees
				seq := p.Revision
				o.Surveillance = &aman.SurveillanceFact{LatitudeDegrees: a.Latitude, LongitudeDegrees: a.Longitude, AltitudeFeet: &alt, GroundspeedKnots: &speed, TrackTrueDegrees: &track, Sequence: &seq, ObservedAt: &observed}
				o.SurveillanceSource = aman.SurveillanceSourceEuroScope
				o.ReconciledAt = observed
				if alt > 1000 && speed > 40 {
					o.TakeoffDetected = &observed
				}
			}
			if p, ok := previous[strip.Callsign][aman.ObservationProviderEuroScope]; ok && p.TakeoffDetected != nil && (o.TakeoffDetected == nil || p.TakeoffDetected.Before(*o.TakeoffDetected)) {
				o.TakeoffDetected = p.TakeoffDetected
			}
			if err = o.Validate(); err != nil {
				return nil, err
			}
			put(o)
		}
	}
	// Retract each source independently. A surviving ES source still owns its
	// surveillance and strip facts when an arrival disappears from VATSIM.
	for callsign, sources := range previous {
		for provider, prior := range sources {
			if !w.options.SourceMode.UsesVATSIM() && provider == aman.ObservationProviderVATSIM || !w.options.SourceMode.UsesEuroScope() && provider == aman.ObservationProviderEuroScope {
				continue
			}
			if _, ok := current[callsign][provider]; ok {
				continue
			}
			prior.Missing = true
			if provider == aman.ObservationProviderVATSIM {
				// An unavailable feed is not evidence of disappearance.
				if result.vatsimStatus != aman.DataFresh {
					prior.Missing = previous[callsign][provider].Missing
				}
				prior.SourceStatus = result.vatsimStatus
				prior.ReconciledAt = result.vatsimAt
				if prior.ReconciledAt.IsZero() {
					prior.ReconciledAt = at
				}
			} else {
				prior.SourceStatus = aman.DataFresh
				prior.ReconciledAt = at
			}
			put(prior)
		}
	}
	callsigns := make([]string, 0, len(current))
	for callsign := range current {
		callsigns = append(callsigns, callsign)
	}
	sort.Strings(callsigns)
	for _, callsign := range callsigns {
		for _, provider := range []aman.ObservationProvider{aman.ObservationProviderVATSIM, aman.ObservationProviderEuroScope, ""} {
			if o, ok := current[callsign][provider]; ok {
				result.observations = append(result.observations, o)
			}
		}
	}
	return result, nil
}
func optionalText(v string) *string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return &v
}

// committedWind reproduces the Open-Meteo grid/hour cache lookup from typed
// accepted pages. Provider fetch and quota reservation belong to its worker.
type committedWind struct {
	state   cluster.NavigationWeather
	airport string
	at      time.Time
}

func (w *committedWind) WindProfile(ctx context.Context, request predictor.WindProfileRequest) (predictor.WindProfile, error) {
	state, err := (cluster.LocalLifecycleStore{Writer: w.state.Writer}).Read(ctx, airportRef(w.airport))
	if err != nil {
		return predictor.WindProfile{}, err
	}
	var pages []*pb.OpenMeteoPage
	for _, e := range state.EntitiesByKind(pb.EntityKind_PROVIDER_CHECKPOINT) {
		c := e.GetValue().GetProviderCheckpoint()
		if c.Provider != "openmeteo" {
			continue
		}
		page, err := w.state.ReadProvider(c.ObjectName, c.Sha256, c.Provider, c.Resource)
		if err != nil {
			return predictor.WindProfile{}, err
		}
		if page.GetOpenMeteo() != nil {
			pages = append(pages, page.GetOpenMeteo())
		}
	}
	sort.Slice(pages, func(i, j int) bool {
		if pages[i].ObservedAt.AsTime().Equal(pages[j].ObservedAt.AsTime()) {
			return pages[i].SourceRevision < pages[j].SourceRevision
		}
		return pages[i].ObservedAt.AsTime().After(pages[j].ObservedAt.AsTime())
	})
	result := predictor.WindProfile{}
	grid := func(v float64) float64 { return math.Round(v/.25) * .25 }
	for _, want := range request.Samples {
		found := false
		for _, p := range pages {
			if p.ObservedAt.AsTime().After(w.at) || !p.ExpiresAt.AsTime().After(w.at) {
				continue
			}
			for _, s := range p.Samples {
				if grid(s.LatitudeDegrees) != grid(want.Position.LatitudeDegrees) || grid(s.LongitudeDegrees) != grid(want.Position.LongitudeDegrees) || !s.ForecastAt.AsTime().Truncate(time.Hour).Equal(want.At.Truncate(time.Hour)) {
					continue
				}
				if result.SourceID == "" {
					result.SourceID = p.SourceId
					result.SourceRevision = p.SourceRevision
					result.ObservedAt = p.ObservedAt.AsTime()
					result.ExpiresAt = p.ExpiresAt.AsTime()
				} else {
					if p.ObservedAt.AsTime().Before(result.ObservedAt) {
						result.ObservedAt = p.ObservedAt.AsTime()
					}
					if p.ExpiresAt.AsTime().Before(result.ExpiresAt) {
						result.ExpiresAt = p.ExpiresAt.AsTime()
					}
				}
				item := predictor.WindSample{Position: want.Position, At: want.At}
				for _, level := range s.Levels {
					item.Levels = append(item.Levels, predictor.WindLevel{AltitudeFeet: level.AltitudeFeet, EastKnots: level.EastKnots, NorthKnots: level.NorthKnots})
				}
				result.Samples = append(result.Samples, item)
				found = true
				break
			}
			if found {
				break
			}
		}
		if !found {
			return predictor.WindProfile{}, fmt.Errorf("committed wind sample unavailable")
		}
	}
	return result, nil
}
