package airportworkflow

import (
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/vatsim"
	"FlightStrips/pkg/events/euroscope"
	"fmt"
	"slices"
	"strings"
)

type ekch struct {
	sequenceFrequencies []string
	definition          *config.AirportDefinition
	airport             string
	geography           shared.BayGeography
	airborneAGL         int64
}

func EKCHDescriptor() Descriptor {
	return Descriptor{ID: "ekch/v1", Bind: func(d *config.AirportDefinition) (Handler, []ActionDescriptor, error) {
		doc := d.Document()
		settings, ok := doc.Workflow.Settings.(config.EKCHSettings)
		if !ok {
			return nil, nil, fmt.Errorf("ekch/v1 requires typed EKCH settings")
		}
		h := ekch{definition: d, airport: d.Airport(), geography: shared.BayGeography{Latitude: doc.Geography.Latitude, Longitude: doc.Geography.Longitude, ElevationFT: int64(doc.Geography.ElevationFT), RelevantDistanceNM: settings.RelevantDistanceNM}, airborneAGL: settings.AirborneAltitudeAGLFT}
		for _, name := range []string{"EKCH_B_GND", "EKCH_DEL"} {
			for _, p := range doc.Positions {
				if p.Name == name {
					h.sequenceFrequencies = append(h.sequenceFrequencies, vatsim.NormalizeFrequency(p.Frequency))
					break
				}
			}
		}
		return h, []ActionDescriptor{Action("strip.move", h.move)}, nil
	}}
}

func (h ekch) move(c Context, a Move) (Decision, error) {
	if a.Bay == shared.BAY_UNKNOWN || a.Bay == shared.BAY_HIDDEN_DEP || a.Bay == shared.BAY_DEP_HIDDEN {
		return Decision{}, fmt.Errorf("invalid bay value: %s", a.Bay)
	}
	if a.ConfirmedRemoval && a.Bay != shared.BAY_HIDDEN {
		return Decision{}, shared.RejectAction("confirmed removal must target the hidden bay")
	}
	if a.Clearance && a.Bay != shared.BAY_CLEARED {
		return Decision{}, shared.RejectAction("clearance moves must target the cleared bay")
	}
	if c.Strip.Bay == shared.BAY_NOT_CLEARED && a.Bay != shared.BAY_NOT_CLEARED && a.Bay != shared.BAY_HIDDEN && !a.Clearance {
		return Decision{}, shared.RejectAction("non-cleared strips cannot be moved out of the not-cleared bay")
	}
	if c.Strip.Destination == h.airport && c.Strip.Origin != h.airport && a.Bay == shared.BAY_NOT_CLEARED {
		return Decision{}, shared.RejectAction("arrival strips cannot be moved to the not-cleared bay")
	}
	d := Decision{Bay: &a.Bay}
	if a.Bay == shared.BAY_NOT_CLEARED || a.Bay == shared.BAY_CLEARED {
		cleared := a.Bay == shared.BAY_CLEARED
		d.Cleared = &cleared
	} else {
		stateDecision, _ := h.moveState(c, a)
		d.GroundState = stateDecision.GroundState
	}
	return d, nil
}

func (h ekch) Observe(c Context, o Observation) (Decision, error) {
	s := c.Strip
	var existing *database.Strip
	if s.Exists {
		existing = &database.Strip{Origin: s.Origin, Destination: s.Destination, Bay: s.Bay, Cleared: s.Cleared, State: nil}
		if s.GroundStateKnown {
			existing.State = &s.GroundState
		}
	}
	bay := s.Bay
	switch o.Kind {
	case FullFlight:
		strip := euroscope.Strip{Origin: o.Origin, Destination: o.Destination, Cleared: o.Cleared, GroundState: o.GroundState, Position: &euroscope.Position{}}
		strip.Position.Lat = o.Latitude
		strip.Position.Lon = o.Longitude
		strip.Position.Altitude = o.Altitude
		bay = shared.GetDepartureBayAt(strip, existing, h.airborneAGL, h.airport, c.GroundOnline, h.geography)
		if existing != nil && !o.Cleared && bay == shared.BAY_NOT_CLEARED && s.Bay != "" && s.Bay != shared.BAY_UNKNOWN && s.Bay != shared.BAY_NOT_CLEARED && (s.PDCState == "CLEARED" || (s.PDCState == "CONFIRMED" && s.Cleared)) {
			bay = s.Bay
		}
	case ArrivalPosition:
		d := Decision{}
		runway, inRunway := h.definition.RunwayRegionAt(o.Latitude, o.Longitude)
		if inRunway && s.Runway != "" && !slices.Contains(runway.Runways, strings.ToUpper(s.Runway)) {
			inRunway = false
		}
		onGround := int64(o.Altitude) < h.geography.ElevationFT+50
		if !s.Landed && s.Runway != "" && (s.Bay == shared.BAY_ARR_HIDDEN || s.Bay == shared.BAY_AIRBORNE) {
			if final, ok := h.definition.FinalRegionAt(s.Runway, o.Latitude, o.Longitude); ok {
				distance := shared.GetDistance(o.Latitude, o.Longitude, final.ThresholdLat, final.ThresholdLon)
				if int64(o.Altitude) <= final.FinalApproachAltitudeCeiling(distance, h.geography.ElevationFT) {
					bay = shared.BAY_FINAL
					d.Bay = &bay
				}
			}
		}
		d.Touchdown = inRunway && onGround && !s.Landed
		if !inRunway && onGround && s.Landed && (s.Bay == shared.BAY_FINAL || s.Bay == shared.BAY_RWY_ARR || s.Bay == shared.BAY_ARR_HIDDEN) {
			bay = shared.BAY_TWY_ARR
			d.Bay = &bay
		}
		return d, nil
	case GroundState:
		if existing == nil {
			return Decision{}, fmt.Errorf("ground-state observation requires a strip")
		}
		bay = shared.GetDepartureBayFromGroundState(o.GroundState, *existing, h.airport, c.GroundOnline)
	case Position:
		if existing == nil {
			return Decision{}, fmt.Errorf("position observation requires a strip")
		}
		bay = shared.GetDepartureBayFromPositionAt(o.Latitude, o.Longitude, int64(o.Altitude), *existing, h.airborneAGL, h.airport, h.geography)
	default:
		return Decision{}, fmt.Errorf("unknown observation kind %d", o.Kind)
	}
	return Decision{Bay: &bay}, nil
}

func (h ekch) Lifecycle(c Context, l Lifecycle) (LifecycleDecision, error) {
	d := LifecycleDecision{Admit: true}
	if !c.Strip.Exists {
		return d, nil
	}
	normalize := func(s string) string { return strings.ToUpper(strings.TrimSpace(s)) }
	airport, origin, destination := normalize(h.airport), normalize(l.Origin), normalize(l.Destination)
	if c.Strip.Bay == shared.BAY_HIDDEN_DEP {
		d.Restart = !c.Strip.EuroscopeSeen && origin == airport && destination != "" && destination != airport &&
			(l.Latitude != 0 || l.Longitude != 0) && shared.GetDistance(l.Latitude, l.Longitude, h.geography.Latitude, h.geography.Longitude) <= h.geography.RelevantDistanceNM && int64(l.Altitude) <= h.geography.ElevationFT+h.airborneAGL
		return d, nil
	}
	d.Restart = normalize(c.Strip.Destination) == airport && origin == airport && destination != "" && destination != airport && (normalize(c.Strip.Origin) != origin || normalize(c.Strip.Destination) != destination)
	return d, nil
}

func (h ekch) Coordinate(c Context, a Coordination) (CoordinationDecision, error) {
	if a.Sequence {
		frequencies := h.sequenceFrequencies
		carriers := make([]string, 0, len(a.Coverage))
		for primary := range a.Coverage {
			carriers = append(carriers, primary)
		}
		slices.Sort(carriers)
		for _, frequency := range frequencies {
			owner := ""
			if _, primed := a.Coverage[frequency]; primed {
				owner = frequency
			} else {
				for _, primary := range carriers {
					if slices.Contains(a.Coverage[primary], frequency) {
						owner = primary
						break
					}
				}
			}
			if owner != "" {
				return CoordinationDecision{SequenceSupported: true, SequenceOwner: owner, SequenceFrequency: frequency}, nil
			}
		}
		return CoordinationDecision{SequenceSupported: true}, nil
	}
	owner := a.SQPosition
	if owner == "" {
		owner = a.DeliveryPosition
	}
	return CoordinationDecision{ClearanceOwner: owner, PromoteTaxi: c.Strip.Bay == shared.BAY_TAXI && c.Strip.Origin == h.airport && c.Strip.Destination != h.airport && a.TargetIsTower, Handover: c.Strip.Bay == shared.BAY_AIRBORNE}, nil
}

func (h ekch) moveState(c Context, a Move) (Decision, error) {
	d := Decision{Bay: &a.Bay}
	if c.Strip.Origin == h.airport {
		state := shared.GetGroundState(a.Bay)
		if state != euroscope.GroundStateUnknown && a.Bay != shared.BAY_STAND {
			d.GroundState = &state
		}
	} else if c.Strip.Destination == h.airport && a.Bay == shared.BAY_STAND {
		state := euroscope.GroundStateParked
		d.GroundState = &state
	}
	return d, nil
}
