package euroscope

import (
	"FlightStrips/internal/aman"
	"fmt"
	"strings"
	"time"
)

func NewAMANGainLossEvent(state aman.AirportState) (AMANGainLossEvent, error) {
	generatedAt, err := aman.FormatTime(state.GeneratedAt)
	if err != nil {
		return AMANGainLossEvent{}, fmt.Errorf("map AMAN gain/loss generated time: %w", err)
	}
	event := AMANGainLossEvent{
		Version: 1, Airport: state.Airport, Revision: uint64(state.Revision), GeneratedAt: generatedAt,
		Authoritative: true, Values: make([]*AMANGainLossValue, 0, len(state.Flights)),
	}
	// Callsign is the AMAN identity. Keep one active projection per normalized
	// callsign so an in-memory transition cannot make EuroScope reject the
	// complete replacement as a duplicate.
	active := make(map[string]aman.AMANFlight, len(state.Flights))
	order := make([]string, 0, len(state.Flights))
	for _, flight := range state.Flights {
		// Commit results retain removed flights for lifecycle processing even
		// though the repository excludes them from subsequent reads.
		callsign := strings.ToUpper(strings.TrimSpace(flight.Callsign))
		if flight.State == aman.StateRemoved {
			continue
		}
		current, exists := active[callsign]
		if !exists {
			order = append(order, callsign)
		}
		if !exists || flight.UpdatedAt.After(current.UpdatedAt) {
			flight.Callsign = callsign
			active[callsign] = flight
		}
	}
	for _, callsign := range order {
		flight := active[callsign]
		value := &AMANGainLossValue{
			Callsign: callsign, DataStatus: string(flight.DataStatus),
			StarFamily: cloneString(flight.SelectedSTARFamily), FeederFix: cloneString(flight.SelectedFeederFix), HoldingFix: cloneString(flight.SelectedHolding),
		}
		if flight.TMAEntry != nil {
			insideTMA := flight.TMAEntry.LastContainment == aman.TMAInside
			value.InsideTma = &insideTMA
		}
		if flight.FeederETA != nil {
			if flight.FeederETA.ETA != nil {
				feederETA, formatErr := aman.FormatTime(*flight.FeederETA.ETA)
				if formatErr != nil {
					return AMANGainLossEvent{}, fmt.Errorf("map AMAN feeder ETA flight %q: %w", flight.Callsign, formatErr)
				}
				value.FeederFixEta = &feederETA
			}
			source, passed := string(flight.FeederETA.Source), flight.FeederETA.Passed
			value.FeederFixEtaSource, value.FeederFixPassed = &source, &passed
		}
		if flight.Prediction != nil && flight.Prediction.Publishable && flight.Slot != nil && flight.Prediction.Calculation != nil && len(flight.Prediction.Calculation.Legs) > 0 {
			referencePoint := strings.TrimSpace(flight.Prediction.Calculation.Legs[len(flight.Prediction.Calculation.Legs)-1].To)
			if referencePoint == "" {
				return AMANGainLossEvent{}, fmt.Errorf("map AMAN gain/loss flight %q: terminal reference point is empty", flight.Callsign)
			}
			// Gain/loss is live guidance against the committed target. The
			// operational TETA may be frozen for sequencing, while RawTETA keeps
			// following the aircraft's current physical trajectory.
			seconds, secondsErr := aman.WholeSeconds(flight.Prediction.RawTETA.Sub(flight.Slot.Time).Round(time.Second))
			if secondsErr != nil {
				return AMANGainLossEvent{}, fmt.Errorf("map AMAN gain/loss flight %q: %w", flight.Callsign, secondsErr)
			}
			targetTime, targetErr := aman.FormatTime(flight.Slot.Time)
			if targetErr != nil {
				return AMANGainLossEvent{}, fmt.Errorf("map AMAN target time flight %q: %w", flight.Callsign, targetErr)
			}
			predictedTime, predictedErr := aman.FormatTime(flight.Prediction.RawTETA)
			if predictedErr != nil {
				return AMANGainLossEvent{}, fmt.Errorf("map AMAN predicted time flight %q: %w", flight.Callsign, predictedErr)
			}
			value.GainLossSeconds = &seconds
			value.ReferencePoint = &referencePoint
			value.TargetTime = &targetTime
			value.PredictedTime = &predictedTime
		}
		event.Values = append(event.Values, value)
	}
	return event, nil
}

func (e AMANGainLossEvent) GetType() EventType { return AMANGainLoss }

func (e AMANGainLossEvent) Marshal() ([]byte, error) { return marshalMessage(&e) }

func (e AMANRouteFactEvent) GetType() EventType { return AMANRouteFact }

func (e AMANRouteFactEvent) Marshal() ([]byte, error) { return marshalMessage(&e) }

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
