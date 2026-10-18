package main

import (
	"FlightStrips/internal/aman"
	"encoding/json"
	"os"
	"time"
)

type occupancySample struct {
	At    time.Time `json:"at"`
	Count int       `json:"count"`
}

// These are planned targets, not inferred controller clearances. API history
// contains surveillance but no authoritative holding/direct clearance facts.
type releaseMetrics struct {
	ObservedOccupancy         []occupancySample     `json:"observed_tma_samples"`
	Snapshots                 int                   `json:"snapshots"`
	PublishableFlights        int                   `json:"publishable_flight_samples"`
	HoldingPlans              int                   `json:"holding_plan_samples"`
	AdvancedPlans             int                   `json:"advanced_plan_samples"`
	HoldingSeconds            float64               `json:"holding_seconds_sum"`
	AdditionalApproachSeconds float64               `json:"additional_approach_seconds_sum"`
	MaxProjectedBuffer        int                   `json:"max_projected_buffer"`
	BufferDegradedSamples     int                   `json:"buffer_degraded_samples"`
	Samples                   []releaseMetricSample `json:"samples"`
}

type releaseMetricSample struct {
	At                time.Time `json:"at"`
	Callsign          string    `json:"callsign"`
	STAR              string    `json:"star"`
	Slot              time.Time `json:"slot"`
	Release           time.Time `json:"release"`
	HoldingSeconds    float64   `json:"holding_seconds"`
	TransitSeconds    float64   `json:"transit_seconds"`
	AdditionalSeconds float64   `json:"additional_approach_seconds"`
}

func (m *releaseMetrics) record(state aman.AirportState, at time.Time) {
	m.Snapshots++
	observed := 0
	for _, f := range state.Flights {
		if f.State == aman.StateLanded || f.State == aman.StateRemoved || f.DataStatus != aman.DataFresh || f.TMAEntry == nil || f.TMAEntry.LastContainment != aman.TMAInside || at.Sub(f.TMAEntry.LastObservedAt) > 2*time.Minute || f.LatestObservation == nil || f.LatestObservation.Surveillance == nil || f.LatestObservation.Surveillance.AltitudeFeet == nil {
			continue
		}
		if *f.LatestObservation.Surveillance.AltitudeFeet > 1500 {
			observed++
		}
	}
	m.ObservedOccupancy = append(m.ObservedOccupancy, occupancySample{At: at, Count: observed})
	if state.ReleaseBuffer != nil {
		m.MaxProjectedBuffer = max(m.MaxProjectedBuffer, state.ReleaseBuffer.Projected())
		if state.ReleaseBuffer.Status == "degraded" {
			m.BufferDegradedSamples++
		}
	}
	for _, f := range state.Flights {
		if f.State == aman.StateLanded || f.State == aman.StateRemoved || f.Prediction == nil || !f.Prediction.Publishable {
			continue
		}
		m.PublishableFlights++
		p := f.Prediction.HoldingPlan
		if p == nil || f.Slot == nil {
			continue
		}
		m.HoldingPlans++
		if p.AdditionalApproachTime > 0 {
			m.AdvancedPlans++
		}
		m.HoldingSeconds += p.ExpectedHoldingDuration.Seconds()
		m.AdditionalApproachSeconds += p.AdditionalApproachTime.Seconds()
		m.Samples = append(m.Samples, releaseMetricSample{At: at, Callsign: f.Callsign, STAR: f.STARFamilyIdentity(), Slot: f.Slot.Time, Release: p.ApproachReleaseTime, HoldingSeconds: p.ExpectedHoldingDuration.Seconds(), TransitSeconds: p.PostHoldingTransit.Seconds(), AdditionalSeconds: p.AdditionalApproachTime.Seconds()})
	}
}

func (m *releaseMetrics) write(path string) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}
