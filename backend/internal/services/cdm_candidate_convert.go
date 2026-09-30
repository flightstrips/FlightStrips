package services

import (
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/cluster"
	"FlightStrips/internal/models"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func cdmClock(t *timestamppb.Timestamp) *string {
	if t == nil {
		return nil
	}
	v := t.AsTime().UTC().Format("1504")
	return &v
}
func cdmSeconds(t *timestamppb.Timestamp) *string { return lifecycleClock(t) }

// CDM is a clock policy; bind clock outputs to the nearest UTC day of the
// accepted work slot. No serialized legacy JSON enters this conversion.
func cdmTimestamp(v *string, anchor time.Time) (*timestamppb.Timestamp, error) {
	if v == nil || strings.TrimSpace(*v) == "" {
		return nil, nil
	}
	format := "1504"
	if len(*v) == 6 {
		format = "150405"
	}
	clock, err := time.Parse(format, *v)
	if err != nil {
		return nil, fmt.Errorf("invalid CDM clock %q", *v)
	}
	at := time.Date(anchor.Year(), anchor.Month(), anchor.Day(), clock.Hour(), clock.Minute(), clock.Second(), 0, time.UTC)
	if at.Sub(anchor) > 12*time.Hour {
		at = at.Add(-24 * time.Hour)
	} else if anchor.Sub(at) > 12*time.Hour {
		at = at.Add(24 * time.Hour)
	}
	return timestamppb.New(at), nil
}

func cdmModel(state *cluster.Aggregate, e *pb.EntitySnapshot, id int32) *models.Strip {
	s := e.GetValue().GetStrip()
	m := lifecycleModelStrip(s, id)
	m.Runway, m.Sid, m.AircraftCategory = &s.Runway, &s.Sid, &s.AircraftCategory
	m.Route, m.Remarks = &s.Route, &s.Remarks
	m.CdmData = &models.CdmData{Eobt: cdmClock(s.Eobt), Tobt: cdmClock(s.Tobt), Tsat: cdmSeconds(s.Tsat), Ttot: cdmSeconds(s.Ttot), Ctot: cdmClock(s.Ctot), Aobt: cdmClock(s.Aobt), Asat: cdmClock(s.Asat), Asrt: cdmClock(s.Asrt), Tsac: cdmClock(s.Tsac), Aldt: cdmClock(s.Aldt), Status: s.OperationalStatus, CtotSource: s.CtotSource, MostPenalizingAirspace: s.MostPenalizingAirspace, EcfmpID: s.EcfmpId, Phase: s.Phase, TobtSetBy: s.TobtSetBy}
	if controller := state.Indexes[pb.EntityKind_CONTROLLER][s.OwnerCid].GetValue().GetController(); controller != nil {
		m.Owner = &controller.Position
	}
	if s.Validation != nil {
		v := s.Validation
		m.ValidationStatus = &models.ValidationStatus{IssueType: v.IssueType, Message: v.Message, OwningPosition: v.OwningPosition, Active: v.Active, ActivationKey: v.ActivationKey}
	}
	if value := state.Indexes[pb.EntityKind_CDM_STATE][s.Callsign].GetValue().GetCdmState(); value != nil {
		d := m.CdmData
		d.Tobt, d.Tsat, d.Ttot, d.Ctot = cdmClock(value.Tobt), cdmSeconds(value.Tsat), cdmSeconds(value.Ttot), cdmClock(value.Ctot)
		d.Atot, d.ViffProposalTsat, d.ViffProposalTtot = cdmClock(value.Atot), cdmSeconds(value.ViffProposalTsat), cdmSeconds(value.ViffProposalTtot)
		d.DeIce = &value.Deice
		d.TobtConfirmedBy, d.TobtAutoSynced, d.TobtAutoAdjusted, d.TobtManuallyConfirmed = value.TobtConfirmedBy, value.TobtAutoSynced, value.TobtAutoAdjusted, value.TobtManuallyConfirmed
		switch value.Recalculation {
		case pb.CdmState_REQUIRED:
			d.MarkLocalRecalculationPending()
		case pb.CdmState_IMPROVE_ONLY:
			d.RecalculationMode = models.CdmRecalculationImproveOnly
			d.Recalculate = true
		}
		d.PushbackRecalculate, d.ReadySyncPending, d.ViffRequestSyncPending, d.AtotViffPending = value.PushbackRecalculate, value.ReadySyncPending, value.ViffRequestSyncPending, value.AtotViffPending
		if calc := value.Calculation; calc != nil {
			d.Calculation = &models.CdmCalculation{BaseTime: cdmSeconds(calc.BaseTime), BaseSource: calc.BaseSource, TaxiMinutes: cdmInt(calc.TaxiMinutes), TaxiRunway: calc.TaxiRunway, InvalidReason: calc.InvalidReason, SequencePosition: cdmInt(calc.SequencePosition), LeaderCallsign: calc.LeaderCallsign, LeaderTtot: cdmSeconds(calc.LeaderTtot)}
			for _, r := range calc.ReasonMarkers {
				d.Calculation.ReasonMarkers = append(d.Calculation.ReasonMarkers, models.CdmReasonMarker{Kind: r.Kind, Message: r.Message, AgainstCallsign: r.AgainstCallsign, AgainstRunway: r.AgainstRunway, AgainstTtot: cdmSeconds(r.AgainstTtot), FromTtot: cdmSeconds(r.FromTtot), ToTtot: cdmSeconds(r.ToTtot), RequiredSpacingMinutes: r.RequiredSpacingMinutes})
			}
		}
	}
	return m
}
func cdmInt(v *int32) *int {
	if v == nil {
		return nil
	}
	n := int(*v)
	return &n
}

func cdmValidation(v *models.ValidationStatus) *pb.ValidationStatus {
	return &pb.ValidationStatus{IssueType: v.IssueType, Message: v.Message, OwningPosition: v.OwningPosition, Active: v.Active, ActivationKey: v.ActivationKey, Action: &pb.ValidationAction{Label: ctotValidationActionLabel, Action: &pb.ValidationAction_AssignHoldingPoint{AssignHoldingPoint: &pb.AssignHoldingPoint{}}}}
}
func cdmInt32(v *int) *int32 {
	if v == nil {
		return nil
	}
	n := int32(*v)
	return &n
}

func cdmRecords(strip *pb.Strip, old *pb.CdmState, data *models.CdmData, anchor time.Time) (*pb.Strip, *pb.CdmState, error) {
	s := proto.Clone(strip).(*pb.Strip)
	c := &pb.CdmState{Callsign: strip.Callsign}
	if old != nil {
		c = proto.Clone(old).(*pb.CdmState)
	}
	var failure error
	stamp := func(v *string) *timestamppb.Timestamp {
		ts, err := cdmTimestamp(v, anchor)
		if err != nil && failure == nil {
			failure = err
		}
		return ts
	}
	s.Eobt, s.Tobt, s.Tsat, s.Ttot, s.Ctot = stamp(data.Eobt), stamp(data.Tobt), stamp(data.Tsat), stamp(data.Ttot), stamp(data.Ctot)
	s.Aobt, s.Asat, s.Asrt, s.Tsac, s.Aldt = stamp(data.Aobt), stamp(data.Asat), stamp(data.Asrt), stamp(data.Tsac), stamp(data.Aldt)
	s.OperationalStatus, s.MostPenalizingAirspace, s.EcfmpId, s.CtotSource, s.Phase, s.TobtSetBy = data.Status, data.MostPenalizingAirspace, data.EcfmpID, data.CtotSource, data.Phase, data.TobtSetBy
	c.Tobt, c.Tsat, c.Ttot, c.Ctot = s.Tobt, s.Tsat, s.Ttot, s.Ctot
	c.Atot, c.ViffProposalTsat, c.ViffProposalTtot = stamp(data.Atot), stamp(data.ViffProposalTsat), stamp(data.ViffProposalTtot)
	// A clock-only policy update must not move unchanged historical actuals
	// onto another UTC day when a later periodic slot executes.
	keep := func(value *string, previous, replacement *timestamppb.Timestamp) *timestamppb.Timestamp {
		if value != nil && previous != nil {
			format := "1504"
			if len(*value) == 6 {
				format = "150405"
			}
			if previous.AsTime().UTC().Format(format) == *value {
				return previous
			}
		}
		return replacement
	}
	s.Eobt, s.Tobt = keep(data.Eobt, strip.Eobt, s.Eobt), keep(data.Tobt, strip.Tobt, s.Tobt)
	s.Aobt, s.Asat = keep(data.Aobt, strip.Aobt, s.Aobt), keep(data.Asat, strip.Asat, s.Asat)
	s.Asrt, s.Tsac, s.Aldt = keep(data.Asrt, strip.Asrt, s.Asrt), keep(data.Tsac, strip.Tsac, s.Tsac), keep(data.Aldt, strip.Aldt, s.Aldt)
	c.Tobt = s.Tobt
	if old != nil {
		c.Atot = keep(data.Atot, old.Atot, c.Atot)
	}
	c.Deice = ""
	if data.DeIce != nil {
		c.Deice = *data.DeIce
	}
	c.Ready = data.Status != nil && *data.Status == "REA"
	c.TobtConfirmedBy, c.TobtAutoSynced, c.TobtAutoAdjusted, c.TobtManuallyConfirmed = data.TobtConfirmedBy, data.TobtAutoSynced, data.TobtAutoAdjusted, data.TobtManuallyConfirmed
	c.Recalculation = pb.CdmState_NONE
	if data.NeedsLocalRecalculation() {
		c.Recalculation = pb.CdmState_REQUIRED
		if data.IsImprovementOnlyRecalculation() {
			c.Recalculation = pb.CdmState_IMPROVE_ONLY
		}
	}
	c.PushbackRecalculate, c.ReadySyncPending, c.ViffRequestSyncPending, c.AtotViffPending = data.PushbackRecalculate, data.ReadySyncPending, data.ViffRequestSyncPending, data.AtotViffPending
	c.Calculation = nil
	if calc := data.Calculation; calc != nil {
		c.Calculation = &pb.CdmState_Calculation{BaseTime: stamp(calc.BaseTime), BaseSource: calc.BaseSource, TaxiMinutes: cdmInt32(calc.TaxiMinutes), TaxiRunway: calc.TaxiRunway, InvalidReason: calc.InvalidReason, SequencePosition: cdmInt32(calc.SequencePosition), LeaderCallsign: calc.LeaderCallsign, LeaderTtot: stamp(calc.LeaderTtot)}
		for _, r := range calc.ReasonMarkers {
			c.Calculation.ReasonMarkers = append(c.Calculation.ReasonMarkers, &pb.CdmState_ReasonMarker{Kind: r.Kind, Message: r.Message, AgainstCallsign: r.AgainstCallsign, AgainstRunway: r.AgainstRunway, AgainstTtot: stamp(r.AgainstTtot), FromTtot: stamp(r.FromTtot), ToTtot: stamp(r.ToTtot), RequiredSpacingMinutes: r.RequiredSpacingMinutes})
		}
	}
	return s, c, failure
}
