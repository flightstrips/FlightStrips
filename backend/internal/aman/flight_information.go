package aman

import "time"

// FlightInitialTiming is additive persisted history, independent of the filed
// airborne baseline used by ETA review. Each clock is captured when first known.
type FlightInitialTiming struct {
	FeederETA *time.Time
	FeederSTA *time.Time
	RunwayETA *time.Time
	RunwaySTA *time.Time
}

func (initial FlightInitialTiming) Validate() error {
	for _, clock := range []struct {
		name  string
		value *time.Time
	}{
		{"initial feeder ETA", initial.FeederETA}, {"initial feeder STA", initial.FeederSTA},
		{"initial runway ETA", initial.RunwayETA}, {"initial runway STA", initial.RunwaySTA},
	} {
		if clock.value != nil {
			if err := requireUTCTime(clock.name, *clock.value); err != nil {
				return err
			}
		}
	}
	return nil
}

// ScheduledFeederTime projects the committed runway slot back through the
// accepted feeder-to-runway transit. A manual ETA is an estimate, not a slot.
// An authoritative holding-derived feeder time already includes the slot.
func ScheduledFeederTime(flight AMANFlight) *time.Time {
	if flight.Slot == nil || flight.Prediction == nil || !flight.Prediction.Publishable {
		return nil
	}
	derived := flight.DerivedFeederETA
	if derived == nil && flight.FeederETA != nil && flight.FeederETA.Source != FeederETASourceManual {
		derived = flight.FeederETA
	}
	if derived != nil && derived.Passed {
		return nil
	}
	if derived != nil && derived.Source == FeederETASourceHolding {
		return copyInformationTime(derived.ETA)
	}
	if flight.SelectedFeederFix == nil || flight.Prediction.Calculation == nil {
		return nil
	}
	found := false
	transit := time.Duration(0)
	for _, leg := range flight.Prediction.Calculation.Legs {
		if found {
			transit += leg.Duration
		}
		if leg.To == *flight.SelectedFeederFix {
			found = true
		}
	}
	if !found {
		return nil
	}
	value := flight.Slot.Time.Add(-transit)
	return &value
}

// CaptureInitialTiming copies the history before adding newly available clocks
// so reconciliation cannot mutate a repository's previously loaded aggregate.
func CaptureInitialTiming(flight *AMANFlight) {
	if flight.Prediction == nil || !flight.Prediction.Publishable || !flight.SequenceDisposition.Participates() || flight.State == StatePlanned || flight.State == StateLanded || flight.State == StateRemoved {
		return
	}
	initial := FlightInitialTiming{}
	if flight.InitialTiming != nil {
		initial = *flight.InitialTiming
	}
	if initial.RunwayETA == nil {
		initial.RunwayETA = copyInformationTime(flight.Prediction.RawRETA)
	}
	if initial.FeederETA == nil && flight.FeederETA != nil {
		initial.FeederETA = copyInformationTime(flight.FeederETA.ETA)
	}
	if initial.RunwaySTA == nil && flight.Slot != nil {
		initial.RunwaySTA = copyInformationTime(&flight.Slot.Time)
	}
	if initial.FeederSTA == nil {
		initial.FeederSTA = ScheduledFeederTime(*flight)
	}
	if initial != (FlightInitialTiming{}) {
		flight.InitialTiming = &initial
	}
}

func copyInformationTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
