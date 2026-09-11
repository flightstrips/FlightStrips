package aman_test

import (
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"github.com/stretchr/testify/require"
)

func TestTypedCommandsValidateOnlyTheirOwnFields(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	meta := aman.CommandMetadata{CommandID: "command-1", ExpectedRevision: 7}
	before, after := aman.FlightID("before"), aman.FlightID("after")

	tests := []struct {
		name    string
		valid   func() error
		invalid func() error
	}{
		{"move", func() error {
			return (aman.MoveFlightCommand{Metadata: meta, FlightID: "flight", RunwayGroupID: "A", BeforeFlightID: &before}).Validate()
		}, func() error {
			return (aman.MoveFlightCommand{Metadata: meta, FlightID: "flight", RunwayGroupID: "A", BeforeFlightID: &before, AfterFlightID: &after}).Validate()
		}},
		{"lock", func() error { return (aman.LockFlightCommand{Metadata: meta, FlightID: "flight"}).Validate() }, func() error { return (aman.LockFlightCommand{Metadata: meta}).Validate() }},
		{"unlock", func() error { return (aman.UnlockFlightCommand{Metadata: meta, FlightID: "flight"}).Validate() }, func() error { return (aman.UnlockFlightCommand{Metadata: meta}).Validate() }},
		{"rate", func() error {
			return (aman.SetRateCommand{Metadata: meta, RunwayGroupID: "A", ArrivalsPerHour: 30, EffectiveAt: now}).Validate()
		}, func() error {
			return (aman.SetRateCommand{Metadata: meta, RunwayGroupID: "A", EffectiveAt: now}).Validate()
		}},
		{"runway selection", func() error {
			return (aman.SelectRunwayGroupCommand{Metadata: meta, RunwayGroupID: "A", EffectiveAt: now}).Validate()
		}, func() error {
			return (aman.SelectRunwayGroupCommand{Metadata: meta, EffectiveAt: now}).Validate()
		}},
		{"active runway groups", func() error {
			return (aman.SetActiveRunwayGroupsCommand{Metadata: meta, RunwayGroupIDs: []aman.RunwayGroupID{"A", "B"}}).Validate()
		}, func() error {
			return (aman.SetActiveRunwayGroupsCommand{Metadata: meta}).Validate()
		}},
		{"accept TETA", func() error { return (aman.AcceptTETACommand{Metadata: meta, FlightID: "flight"}).Validate() }, func() error { return (aman.AcceptTETACommand{Metadata: meta}).Validate() }},
		{"keep FPL ETA", func() error { return (aman.KeepFPLETACommand{Metadata: meta, FlightID: "flight"}).Validate() }, func() error { return (aman.KeepFPLETACommand{Metadata: meta}).Validate() }},
		{"manual ETA", func() error {
			return (aman.SetManualETACommand{Metadata: meta, FlightID: "flight", ManualETA: now.Add(time.Minute)}).Validate(now)
		}, func() error {
			return (aman.SetManualETACommand{Metadata: meta, FlightID: "flight", ManualETA: now}).Validate(now)
		}},
		{"reset TETA", func() error { return (aman.ResetTETAOverrideCommand{Metadata: meta, FlightID: "flight"}).Validate() }, func() error { return (aman.ResetTETAOverrideCommand{Metadata: meta}).Validate() }},
		{"manual feeder ETA", func() error {
			return (aman.SetManualFeederETACommand{Metadata: meta, FlightID: "flight", FeederETA: now}).Validate()
		}, func() error {
			return (aman.SetManualFeederETACommand{Metadata: meta, FlightID: "flight", FeederETA: now.In(time.FixedZone("CEST", 2*60*60))}).Validate()
		}},
		{"reset manual feeder ETA", func() error {
			return (aman.ResetManualFeederETACommand{Metadata: meta, FlightID: "flight"}).Validate()
		}, func() error { return (aman.ResetManualFeederETACommand{Metadata: meta}).Validate() }},
		{"go around", func() error {
			return (aman.ReportGoAroundCommand{Metadata: meta, FlightID: "flight", DetectedAt: now.Add(-time.Second)}).Validate(now)
		}, func() error {
			return (aman.ReportGoAroundCommand{Metadata: meta, FlightID: "flight", DetectedAt: now.Add(time.Second)}).Validate(now)
		}},
		{"confirm go around", func() error {
			return (aman.ConfirmGoAroundCommand{Metadata: meta, FlightID: "flight", EpisodeID: "flight/go-around/1"}).Validate()
		}, func() error {
			return (aman.ConfirmGoAroundCommand{Metadata: meta, FlightID: "flight"}).Validate()
		}},
		{"reject go around", func() error {
			return (aman.RejectGoAroundCommand{Metadata: meta, FlightID: "flight", EpisodeID: "flight/go-around/1"}).Validate()
		}, func() error {
			return (aman.RejectGoAroundCommand{Metadata: meta, FlightID: "flight", EpisodeID: " go-around "}).Validate()
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, test.valid())
			var domain *aman.DomainError
			require.ErrorAs(t, test.invalid(), &domain)
			require.Equal(t, aman.ErrorInvalidArgument, domain.Class)
		})
	}
}

func TestSetActiveRunwayGroupsRejectsDuplicateIDs(t *testing.T) {
	command := aman.SetActiveRunwayGroupsCommand{
		Metadata:       aman.CommandMetadata{CommandID: "set-runways", ExpectedRevision: 7},
		RunwayGroupIDs: []aman.RunwayGroupID{"A", "A"},
	}
	var domain *aman.DomainError
	require.ErrorAs(t, command.Validate(), &domain)
	require.Equal(t, aman.ErrorInvalidArgument, domain.Class)
}

func TestCommandContextRequiresServerDerivedAuthorityAndUTCReceipt(t *testing.T) {
	valid := aman.CommandContext{Airport: "EKCH", Actor: "1234567", Role: "EKCH_FMH", ReceivedAt: time.Now().UTC()}
	require.NoError(t, valid.Validate())

	for _, invalid := range []aman.CommandContext{
		{Actor: valid.Actor, Role: valid.Role, ReceivedAt: valid.ReceivedAt},
		{Airport: valid.Airport, Role: valid.Role, ReceivedAt: valid.ReceivedAt},
		{Airport: valid.Airport, Actor: valid.Actor, ReceivedAt: valid.ReceivedAt},
		{Airport: valid.Airport, Actor: valid.Actor, Role: valid.Role, ReceivedAt: time.Now()},
	} {
		require.Error(t, invalid.Validate())
	}
}
