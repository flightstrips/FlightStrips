package airportworkflow

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/shared"
	pkg "FlightStrips/pkg/models"
	"fmt"
	"math"
	"regexp"
	"time"
)

type radis struct {
	airport                        string
	latitude, longitude, elevation float64
	settings                       config.RADISSettings
}

func RADISDescriptor() Descriptor {
	return Descriptor{ID: "radis/v1", BoardState: func(d *config.AirportDefinition) BoardStatePolicy {
		return BoardStatePolicy{Revisioned: true, RecoverySeconds: d.Document().Workflow.Settings.(config.RADISSettings).RecoverySeconds}
	}, Bind: func(d *config.AirportDefinition) (Handler, []ActionDescriptor, error) {
		doc := d.Document()
		s, ok := doc.Workflow.Settings.(config.RADISSettings)
		if !ok {
			return nil, nil, fmt.Errorf("RADIS requires typed settings")
		}
		h := radis{d.Airport(), doc.Geography.Latitude, doc.Geography.Longitude, doc.Geography.ElevationFT, s}
		return h, []ActionDescriptor{Action("strip.annotate", h.annotate), Action("strip.create-local", h.createLocal), Action("flight.find-connected", h.findConnected), Action("strip.admit-connected", h.admitConnected), Action("strip.remove", h.remove), Action("strip.restore", h.restore), Action("strip.move", h.move), Action("strip.state-for-move", h.move)}, nil
	}}
}
func (h radis) initial(origin string) string {
	if origin == h.airport {
		return h.settings.DepartureInitialBay
	}
	return h.settings.ArrivalInitialBay
}
func (h radis) move(c Context, a Move) (Decision, error) {
	states, ok := h.settings.BayGroundStates[a.Bay]
	if !ok || a.Clearance || a.ConfirmedRemoval {
		return Decision{}, shared.RejectAction("invalid RADIS move")
	}
	if c.Strip.Bay == a.Bay {
		return Decision{Bay: &a.Bay}, nil
	}
	if c.Strip.Local && !c.Strip.HasFP {
		return Decision{Bay: &a.Bay}, nil
	}
	if !c.Strip.EuroscopeSeen || !c.Strip.HasFP {
		return Decision{}, shared.RejectAction("an associated EuroScope flight is required for this bay effect")
	}
	state := states.Arrival
	if c.Strip.Origin == h.airport {
		state = states.Departure
	}
	return Decision{Bay: &a.Bay, GroundState: &state}, nil
}
func (h radis) Observe(c Context, o Observation) (Decision, error) {
	bay := c.Strip.Bay
	if !c.Strip.Exists || !c.Strip.BoardMember {
		bay = h.initial(o.Origin)
	}
	// ES observations update flight facts, never overwrite explicit board placement
	// or echo a controller command back to ES.
	d := Decision{Bay: &bay}
	if o.Kind == FullFlight || o.Kind == Position {
		d.Visibility = h.visibility(c, o)
	}
	if o.Kind == BackendState && c.Strip.GroundStateKnown {
		d.ReconciledGroundState = &c.Strip.GroundState
	}
	if o.Kind == FullFlight || o.Kind == GroundState {
		d.ReconciledGroundState = &o.GroundState
	}
	if o.Kind == FullFlight || o.Kind == ClearedFlag {
		d.ReconciledCleared = &o.Cleared
	}
	return d, nil
}
func (h radis) Lifecycle(c Context, l Lifecycle) (LifecycleDecision, error) {
	if l.Kind == Removal {
		delay := time.Duration(h.settings.DisconnectRemovalSeconds * float64(time.Second))
		d := LifecycleDecision{RemovalDelay: &delay}
		if c.Strip.BoardMember && (c.Strip.EuroscopeSeen || c.Strip.HasFP) {
			d.Visibility = &StripVisibility{Reason: l.Reason}
		}
		return d, nil
	} // retained recovery state; never hard-delete an airport strip
	if l.Kind == BackendAdmission {
		return LifecycleDecision{Admit: c.Strip.Exists && c.Strip.BoardMember && c.Strip.EuroscopeSeen && c.Strip.HasFP && c.Strip.RemovedAt == nil}, nil
	}
	if c.Strip.Exists && c.Strip.BoardMember {
		return LifecycleDecision{Admit: true}, nil
	}
	relevant := l.Origin == h.airport || l.Destination == h.airport
	distance := RADISDistanceNM(h.latitude, h.longitude, l.Latitude, l.Longitude)
	admit := relevant && distance <= h.settings.AdmissionDistanceNM && (l.Origin != h.airport || float64(l.Altitude)-h.elevation <= float64(h.settings.DepartureAdmissionAGLFT))
	bay := h.initial(l.Origin)
	return LifecycleDecision{Admit: admit, Bay: &bay}, nil
}
func (radis) Coordinate(Context, Coordination) (CoordinationDecision, error) {
	return CoordinationDecision{}, nil
}

func RADISDistanceNM(lat1, lon1, lat2, lon2 float64) float64 {
	r := math.Pi / 180
	a := math.Pow(math.Sin((lat2-lat1)*r/2), 2) + math.Cos(lat1*r)*math.Cos(lat2*r)*math.Pow(math.Sin((lon2-lon1)*r/2), 2)
	return 3440.065 * 2 * math.Asin(math.Sqrt(math.Min(1, math.Max(0, a))))
}

// StripAnnotation is a semantic paper-field edit, never an ES amendment.

func (h radis) annotate(c Context, a StripAnnotation) (Decision, error) {
	v := c.Strip.Annotations
	stamp := func() *string {
		if !a.Enabled {
			return nil
		}
		t := c.Now.UTC().Format(time.RFC3339)
		return &t
	}
	switch a.Field {
	case "notes":
		if len(a.Value) > 2048 {
			return Decision{}, fmt.Errorf("notes too long")
		}
		v.Notes = a.Value
	case "deicing":
		v.Deicing = a.Enabled
	case "rea":
		if c.Now.IsZero() {
			return Decision{}, fmt.Errorf("server clock required")
		}
		v.REA = stamp()
	case "ata":
		if c.Now.IsZero() {
			return Decision{}, fmt.Errorf("server clock required")
		}
		v.ATA = stamp()
	case "atd":
		if a.Value != "" && !regexp.MustCompile(`^(?:[01][0-9]|2[0-3])[0-5][0-9]$`).MatchString(a.Value) {
			return Decision{}, fmt.Errorf("ATD must be HHMM UTC")
		}
		v.ATD = a.Value
	case "endurance_pob":
		if len(a.Value) > 16 || !regexp.MustCompile(`^[0-9./]*$`).MatchString(a.Value) {
			return Decision{}, fmt.Errorf("invalid endurance/POB")
		}
		v.EndurancePOB = a.Value
	case "qnh":
		if a.Value != "" && !regexp.MustCompile(`^Q[0-9]{4}$`).MatchString(a.Value) {
			return Decision{}, fmt.Errorf("invalid QNH stamp")
		}
		v.QNH = a.Value
	case "rnav":
		if a.Value != "" && a.Value != "1" && a.Value != "NON" {
			return Decision{}, fmt.Errorf("invalid RNAV mark")
		}
		v.RNAV = a.Value
	case "vfr":
		v.VFR = a.Enabled
	default:
		return Decision{}, fmt.Errorf("unknown RADIS annotation")
	}
	return Decision{Annotations: &v}, nil
}

func (h radis) visibility(c Context, o Observation) *StripVisibility {
	if !c.Strip.BoardMember || c.Strip.Local && !c.Strip.HasFP {
		return nil
	}
	origin, destination := o.Origin, o.Destination
	if origin == "" {
		origin = c.Strip.Origin
	}
	if destination == "" {
		destination = c.Strip.Destination
	}
	distance := RADISDistanceNM(h.latitude, h.longitude, o.Latitude, o.Longitude)
	agl := float64(o.Altitude) - h.elevation
	if c.Strip.RemovedAt != nil {
		if c.Strip.RemovalReason != "manual" && h.settings.AutomaticReappearance && !c.Now.IsZero() && !c.Now.Before(*c.Strip.RemovedAt) && c.Now.Before(c.Strip.RemovedAt.Add(time.Duration(h.settings.RecoverySeconds)*time.Second)) && (origin == h.airport || destination == h.airport) && distance <= h.settings.AdmissionDistanceNM && (origin != h.airport || agl <= float64(h.settings.DepartureAdmissionAGLFT)) {
			return &StripVisibility{Visible: true}
		}
		return nil
	}
	if origin == h.airport && agl >= float64(h.settings.DepartureRemovalAGLFT) {
		return &StripVisibility{Reason: "departure-altitude"}
	}
	if distance > h.settings.RemovalDistanceNM {
		return &StripVisibility{Reason: "distance"}
	}
	return nil
}

func (h radis) createLocal(c Context, r pkg.LocalStripCreation) (Decision, error) {
	return Decision{Admission: &StripAdmission{Bay: h.initial(r.Origin), Annotations: pkg.StripAnnotations{Notes: r.Notes}}}, nil
}
func (h radis) findConnected(c Context, r ConnectedFlightSearch) (Decision, error) {
	return Decision{ConnectedFlights: &ConnectedFlightLookup{Airport: h.airport, Query: r.Query}}, nil
}
func (h radis) admitConnected(c Context, r pkg.ConnectedFlightCandidate) (Decision, error) {
	return Decision{Admission: &StripAdmission{Bay: h.initial(r.Origin)}}, nil
}
func (h radis) remove(c Context, _ StripRemoval) (Decision, error) {
	if c.Strip.RemovedAt != nil {
		return Decision{}, nil
	}
	return Decision{Visibility: &StripVisibility{Reason: "manual"}}, nil
}
func (h radis) restore(c Context, _ StripRestoration) (Decision, error) {
	if c.Strip.RemovedAt == nil {
		return Decision{}, nil
	}
	if c.Now.Before(*c.Strip.RemovedAt) || !c.Now.Before(c.Strip.RemovedAt.Add(time.Duration(h.settings.RecoverySeconds)*time.Second)) {
		return Decision{}, shared.RejectAction("airport strip recovery period has expired")
	}
	return Decision{Visibility: &StripVisibility{Visible: true}}, nil
}
