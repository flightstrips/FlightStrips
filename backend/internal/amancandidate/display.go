package amancandidate

import (
	"FlightStrips/internal/aman"
	pb "FlightStrips/pkg/events/cluster"
	"FlightStrips/pkg/events/frontend"
)

func displayAMANHeader(v frontend.AMANHeader) *pb.AmanDisplayHeader {
	r := &pb.AmanDisplayHeader{}
	for _, x := range v.ActiveRunwayGroups {
		r.ActiveRunwayGroups = append(r.ActiveRunwayGroups, displayAMANHeaderRunwayGroup(x))
	}
	r.Readiness = displayAMANHeaderReadiness(v.Readiness)
	r.TrafficSummary = displayAMANHeaderTrafficSummary(v.TrafficSummary)
	if v.Wind != nil {
		r.Wind = displayAMANHeaderWind(*v.Wind)
	}
	return r
}
func displayAMANHeaderRunwayGroup(v frontend.AMANHeaderRunwayGroup) *pb.AmanDisplayHeaderRunwayGroup {
	r := &pb.AmanDisplayHeaderRunwayGroup{}
	r.Id = v.ID
	r.ActiveRatePerHour = v.ActiveRatePerHour
	r.RateEffectiveAt = v.RateEffectiveAt
	return r
}
func displayAMANHeaderReadiness(v frontend.AMANHeaderReadiness) *pb.AmanDisplayHeaderReadiness {
	r := &pb.AmanDisplayHeaderReadiness{}
	r.Status = v.Status
	r.Ready = v.Ready
	r.BlockedReasons = append([]string(nil), v.BlockedReasons...)
	return r
}
func displayAMANHeaderTrafficSummary(v frontend.AMANHeaderTrafficSummary) *pb.AmanDisplayHeaderTrafficSummary {
	r := &pb.AmanDisplayHeaderTrafficSummary{}
	r.Status = v.Status
	r.TmaAbove_1500FeetCount = int32(v.TMAAbove1500FeetCount)
	r.MaestroHorizonCount = int32(v.MaestroHorizonCount)
	return r
}
func displayAMANHeaderWind(v frontend.AMANHeaderWind) *pb.AmanDisplayHeaderWind {
	r := &pb.AmanDisplayHeaderWind{}
	r.SurfaceDirectionDegrees = int32(v.SurfaceDirectionDegrees)
	r.SurfaceSpeedKnots = int32(v.SurfaceSpeedKnots)
	r.Direction_10000Degrees = int32(v.Direction10000Degrees)
	r.Speed_10000Knots = int32(v.Speed10000Knots)
	r.ObservedAt = v.ObservedAt
	r.Source = v.Source
	return r
}
func displayAMANTrafficPrediction(v frontend.AMANTrafficPrediction) *pb.AmanDisplayTrafficPrediction {
	r := &pb.AmanDisplayTrafficPrediction{}
	r.GeneratedAt = v.GeneratedAt
	r.RangeStart = v.RangeStart
	r.RangeEnd = v.RangeEnd
	r.BucketMinutes = int32(v.BucketMinutes)
	r.SourceStatus = v.SourceStatus
	r.Status = v.Status
	r.DegradedReasons = append([]string(nil), v.DegradedReasons...)
	for _, x := range v.Buckets {
		r.Buckets = append(r.Buckets, displayAMANTrafficBucket(x))
	}
	return r
}
func displayAMANTrafficBucket(v frontend.AMANTrafficBucket) *pb.AmanDisplayTrafficBucket {
	r := &pb.AmanDisplayTrafficBucket{}
	r.Start = v.Start
	r.End = v.End
	r.PlannedCount = int32(v.PlannedCount)
	r.AirborneCount = int32(v.AirborneCount)
	r.Count = int32(v.Count)
	r.LoadFactor = int32(v.LoadFactor)
	if v.SelectedRate != nil {
		r.SelectedRate = displayAMANTrafficSelectedRate(*v.SelectedRate)
	}
	r.BucketHigh = v.BucketHigh
	r.WindowHigh = v.WindowHigh
	r.Alert = v.Alert
	for _, x := range v.Flights {
		r.Flights = append(r.Flights, displayAMANTrafficFlight(x))
	}
	return r
}
func displayAMANTrafficSelectedRate(v frontend.AMANTrafficSelectedRate) *pb.AmanDisplayTrafficSelectedRate {
	r := &pb.AmanDisplayTrafficSelectedRate{}
	r.RunwayGroupId = v.RunwayGroupID
	r.ArrivalsPerHour = v.ArrivalsPerHour
	r.EffectiveAt = v.EffectiveAt
	return r
}
func displayAMANTrafficFlight(v frontend.AMANTrafficFlight) *pb.AmanDisplayTrafficFlight {
	r := &pb.AmanDisplayTrafficFlight{}
	r.Callsign = v.Callsign
	r.Airborne = v.Airborne
	r.LandingAt = v.LandingAt
	r.TimingSource = v.TimingSource
	r.DataStatus = v.DataStatus
	return r
}
func displayAMANHoldingEntry(v frontend.AMANHoldingEntry) *pb.AmanDisplayHoldingEntry {
	r := &pb.AmanDisplayHoldingEntry{}
	r.Callsign = v.Callsign
	r.Holding = v.Holding
	r.Eat = v.EAT
	r.ClearedAltitude = v.ClearedAltitude
	r.SourceStatus = v.SourceStatus
	r.ObservedAt = v.ObservedAt
	return r
}
func displayAMANWarning(v frontend.AMANWarning) *pb.AmanDisplayWarning {
	r := &pb.AmanDisplayWarning{}
	r.Id = v.ID
	r.Source = v.Source
	r.Component = v.Component
	r.Severity = v.Severity
	r.Code = v.Code
	r.RunwayGroupId = v.RunwayGroupID
	r.Callsign = v.Callsign
	r.RelatedCallsign = v.RelatedCallsign
	r.Message = v.Message
	return r
}

// attachDisplay runs the existing backend read-model policy before persistence.
// Binary clients receive typed projections; no client reconstructs traffic policy.
func attachDisplay(a *pb.AmanAirport, state aman.AirportState, health aman.TechnicalHealth) error {
	event, err := frontend.NewAMANStateEvent(state, health.EffectiveMode, health)
	if err != nil {
		return err
	}
	a.Header = displayAMANHeader(*event.Data.Header)
	a.TrafficPrediction = displayAMANTrafficPrediction(event.Data.TrafficPrediction)
	for _, v := range event.Data.HoldingInformation {
		a.HoldingInformation = append(a.HoldingInformation, displayAMANHoldingEntry(v))
	}
	for _, v := range event.Data.Warnings {
		a.Warnings = append(a.Warnings, displayAMANWarning(v))
	}
	return nil
}

// refreshDisplay retains accepted technical health while projecting the new command state.
func refreshDisplay(a *pb.AmanAirport, state aman.AirportState, prior *pb.AmanAirport) error {
	h := aman.TechnicalHealth{DesiredMode: state.Mode, EffectiveMode: aman.EffectiveRolloutMode(prior.EffectiveMode), Ready: prior.GetHealth().GetReady(), Status: aman.HealthStatus(prior.GetHealth().GetStatus()), BlockedReasons: prior.GetHealth().GetBlockedReasons()}
	for _, v := range prior.GetHealth().GetComponents() {
		c := aman.ComponentHealth{Status: aman.HealthStatus(v.Status), Reason: v.Reason, UpdatedAt: optionalInstant(v.UpdatedAt), AgeSeconds: v.AgeSeconds}
		switch v.Component {
		case "observation_source":
			h.ObservationSource = c
			h.VATSIM = c
		case "navigation":
			h.Navigation = c
		case "weather":
			h.Weather = c
		case "repository":
			h.Repository = c
		case "predictor":
			h.Predictor = c
		case "replay_validation":
			h.ReplayValidation = c
		}
	}
	a.EffectiveMode = prior.EffectiveMode
	return attachDisplay(a, state, h)
}
