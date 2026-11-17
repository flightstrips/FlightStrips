package aman

import "time"

// ApproachReleasePolicy separates approach admission from runway capacity.
// Aircraft counts are desired buffer settings, not airspace occupancy limits.
type ApproachReleasePolicy struct {
	Enabled           bool  `json:"enabled"`
	TargetBuffer      int   `json:"targetBuffer"`
	TaperStart        int   `json:"taperStart"`
	MaxAdvanceSeconds int64 `json:"maxAdvanceSeconds"`
	LookaheadSeconds  int64 `json:"lookaheadSeconds"`
}

func (p ApproachReleasePolicy) Validate() error {
	if p.TargetBuffer <= 0 || p.TaperStart <= 0 || p.TaperStart >= p.TargetBuffer || p.MaxAdvanceSeconds <= 0 || p.LookaheadSeconds <= 0 {
		return invalid("approach release policy requires positive settings and taper start below target")
	}
	if p.MaxAdvanceSeconds > int64((24*time.Hour)/time.Second) || p.LookaheadSeconds > int64((time.Hour)/time.Second) {
		return invalid("approach release policy durations are out of range")
	}
	return nil
}

// ApproachEntryEstimate describes physical entry into the TMA, even when a
// landing ETA spike is withheld from operational guidance.
type ApproachEntryEstimate struct {
	ObservedAt     time.Time
	EntryAt        time.Time
	GeometryDigest string
	RunwayGroupID  RunwayGroupID
}

func (e ApproachEntryEstimate) Validate() error {
	if e.GeometryDigest == "" || e.RunwayGroupID == "" || e.EntryAt.Before(e.ObservedAt) {
		return invalid("approach entry estimate is incomplete")
	}
	if err := requireUTCTime("approach entry observation", e.ObservedAt); err != nil {
		return err
	}
	return requireUTCTime("predicted approach entry", e.EntryAt)
}

// ApproachReleaseReservation survives EAT expiry, source outages and restart.
// ReleasedAt requires physical evidence; a cancelled hold is insufficient.
type ApproachReleaseReservation struct {
	Episode                string
	HoldingID              string
	STARFamily             string
	FeederFix              string
	RunwayGroupID          RunwayGroupID
	HoldingEntryAt         time.Time
	ReleaseAt              time.Time
	ReleasedAt             *time.Time
	Transit                time.Duration
	AdditionalApproachTime time.Duration
	RouteDigest            string
	PolicyVersion          string
}

// ApproachGuidancePlan is a replaceable preview, never an admission or EAT.
// Bind it to the prediction, route and landing slot that produced it so a
// changed input cannot reuse a former downstream absorption allowance.
type ApproachGuidancePlan struct {
	HoldingID     string
	RunwayGroupID RunwayGroupID
	RouteDigest   string
	SlotAt        time.Time
	PredictionAt  time.Time
	Plan          HoldingPlan
}

func (p ApproachGuidancePlan) Validate() error {
	if p.HoldingID == "" || p.RunwayGroupID == "" || p.RouteDigest == "" {
		return invalid("approach guidance identity is incomplete")
	}
	if err := requireUTCTime("approach guidance slot", p.SlotAt); err != nil {
		return err
	}
	if err := requireUTCTime("approach guidance prediction", p.PredictionAt); err != nil {
		return err
	}
	if err := p.Plan.Validate(); err != nil {
		return err
	}
	if p.SlotAt.Sub(p.Plan.ApproachReleaseTime)-p.Plan.PostHoldingTransit != p.Plan.AdditionalApproachTime {
		return invalid("approach guidance absorption does not match its landing slot")
	}
	return nil
}

func (r ApproachReleaseReservation) Validate() error {
	if r.Episode == "" || r.HoldingID == "" || r.STARFamily == "" || r.FeederFix == "" || r.RunwayGroupID == "" || r.Transit <= 0 || r.AdditionalApproachTime < 0 || r.RouteDigest == "" || r.PolicyVersion == "" {
		return invalid("approach release reservation is incomplete")
	}
	if err := requireUTCTime("approach reservation release", r.ReleaseAt); err != nil {
		return err
	}
	if !r.HoldingEntryAt.IsZero() {
		if err := requireUTCTime("reservation holding entry", r.HoldingEntryAt); err != nil {
			return err
		}
		if !r.ReleaseAt.After(r.HoldingEntryAt) {
			return invalid("reservation release precedes holding entry")
		}
	}
	if r.ReleasedAt != nil {
		if r.ReleasedAt.Before(r.ReleaseAt) {
			return invalid("observed release precedes its accepted target")
		}
		return requireUTCTime("observed approach release", *r.ReleasedAt)
	}
	return nil
}

type ReleaseBufferSummary struct {
	BlockedReasons   []string
	UnknownCallsigns []Callsign
	// AdvisoryReasons describe recoverable timing mismatches. Admissions
	// remain capacity consuming and independent intake can continue.
	AdvisoryReasons []string
	Status          string
	Target          int
	InApproach      int
	Reserved        int
	Incoming        int
	Unknown         int
}

func (s ReleaseBufferSummary) Projected() int { return s.InApproach + s.Reserved + s.Incoming }

func (s ReleaseBufferSummary) Validate() error {
	if s.Status != "ready" && s.Status != "degraded" && s.Status != "disabled" {
		return invalid("release buffer status is invalid")
	}
	if s.Target <= 0 || s.InApproach < 0 || s.Reserved < 0 || s.Incoming < 0 || s.Unknown < 0 {
		return invalid("release buffer counts are invalid")
	}
	return nil
}
