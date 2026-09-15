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
	// Existing state may still contain duplicate active rows during migration.
	// Publish only the most recently updated flight per network callsign, with
	// a stable identity tie-breaker so slice order cannot change the result.
	selected := make(map[string]int, len(state.Flights))
	for index, flight := range state.Flights {
		if flight.State == aman.StateRemoved {
			continue
		}
		callsign := strings.ToUpper(strings.TrimSpace(flight.CurrentCallsign))
		previous, exists := selected[callsign]
		if !exists || flight.UpdatedAt.After(state.Flights[previous].UpdatedAt) ||
			(flight.UpdatedAt.Equal(state.Flights[previous].UpdatedAt) && flight.ID > state.Flights[previous].ID) {
			selected[callsign] = index
		}
	}
	for index, flight := range state.Flights {
		// Commit results retain removed flights for lifecycle processing even
		// though the repository excludes them from subsequent reads. Match that
		// active projection: a retired callsign can belong to a new identity,
		// and older EuroScope plugins reject duplicate callsigns outright.
		callsign := strings.ToUpper(strings.TrimSpace(flight.CurrentCallsign))
		if flight.State == aman.StateRemoved || selected[callsign] != index {
			continue
		}
		value := &AMANGainLossValue{
			FlightId: string(flight.ID), Callsign: callsign, DataStatus: string(flight.DataStatus),
			StarFamily: cloneString(flight.SelectedSTARFamily), FeederFix: cloneString(flight.SelectedFeederFix), HoldingFix: cloneString(flight.SelectedHolding),
		}
		if flight.FeederETA != nil {
			if flight.FeederETA.ETA != nil {
				feederETA, formatErr := aman.FormatTime(*flight.FeederETA.ETA)
				if formatErr != nil {
					return AMANGainLossEvent{}, fmt.Errorf("map AMAN feeder ETA flight %q: %w", flight.ID, formatErr)
				}
				value.FeederFixEta = &feederETA
			}
			source, passed := string(flight.FeederETA.Source), flight.FeederETA.Passed
			value.FeederFixEtaSource, value.FeederFixPassed = &source, &passed
		}
		if flight.Prediction != nil && flight.Prediction.Publishable && flight.Slot != nil && flight.Prediction.Calculation != nil && len(flight.Prediction.Calculation.Legs) > 0 {
			referencePoint := strings.TrimSpace(flight.Prediction.Calculation.Legs[len(flight.Prediction.Calculation.Legs)-1].To)
			if referencePoint == "" {
				return AMANGainLossEvent{}, fmt.Errorf("map AMAN gain/loss flight %q: terminal reference point is empty", flight.ID)
			}
			seconds, secondsErr := aman.WholeSeconds(flight.Prediction.OperationalTETA.Sub(flight.Slot.Time).Round(time.Second))
			if secondsErr != nil {
				return AMANGainLossEvent{}, fmt.Errorf("map AMAN gain/loss flight %q: %w", flight.ID, secondsErr)
			}
			targetTime, targetErr := aman.FormatTime(flight.Slot.Time)
			if targetErr != nil {
				return AMANGainLossEvent{}, fmt.Errorf("map AMAN target time flight %q: %w", flight.ID, targetErr)
			}
			predictedTime, predictedErr := aman.FormatTime(flight.Prediction.OperationalTETA)
			if predictedErr != nil {
				return AMANGainLossEvent{}, fmt.Errorf("map AMAN predicted time flight %q: %w", flight.ID, predictedErr)
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
