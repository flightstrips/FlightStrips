package models

import "strings"

const (
	CtotSourceManual = "Manual"
	CtotSourceEvent  = "EVENT"
	CtotSourceATFCM  = "ATFCM"

	TobtConfirmedByATC   = "ATC"
	TobtConfirmedByPilot = "Pilot"

	CdmCalculationBaseTobt    = "TOBT"
	CdmCalculationBaseReqTobt = "REQ_TOBT"
	CdmCalculationBaseEobt    = "EOBT"

	CdmInvalidReasonStaleTobt = "STALE_TOBT"
	CdmInvalidReasonStaleTsat = "STALE_TSAT"
)

// CdmCalculation captures the last local sequencing snapshot used to derive
// TSAT/TTOT and invalidation state. It is internal persistence, not a wire DTO.
type CdmCalculation struct {
	BaseTime      *string `json:"baseTime,omitempty"`
	BaseSource    *string `json:"baseSource,omitempty"`
	TaxiMinutes   *int    `json:"taxiMinutes,omitempty"`
	TaxiRunway    *string `json:"taxiRunway,omitempty"`
	InvalidReason *string `json:"invalidReason,omitempty"`
}

type CdmData struct {
	Tobt            *string         `json:"tobt,omitempty"`
	TobtSetBy       *string         `json:"tobtSetBy,omitempty"`
	TobtConfirmedBy *string         `json:"tobtConfirmedBy,omitempty"`
	ReqTobt         *string         `json:"reqTobt,omitempty"`
	ReqTobtType     *string         `json:"reqTobtType,omitempty"`
	Tsat            *string         `json:"tsat,omitempty"`
	Ttot            *string         `json:"ttot,omitempty"`
	Ctot            *string         `json:"ctot,omitempty"`
	CtotSource      *string         `json:"ctotSource,omitempty"`
	Aobt            *string         `json:"aobt,omitempty"`
	Asat            *string         `json:"asat,omitempty"`
	Asrt            *string         `json:"asrt,omitempty"`
	Tsac            *string         `json:"tsac,omitempty"`
	Eobt            *string         `json:"eobt,omitempty"`
	Aldt            *string         `json:"aldt,omitempty"`
	Status          *string         `json:"status,omitempty"`
	DeIce           *string         `json:"deIce,omitempty"`
	EcfmpID         *string         `json:"ecfmpId,omitempty"`
	Phase           *string         `json:"phase,omitempty"`
	Calculation     *CdmCalculation `json:"calculation,omitempty"`
	Recalculate     bool            `json:"recalculate,omitempty"`
}

type CdmDataRow struct {
	Callsign string
	Data     *CdmData
}

// NewLegacyCdmData creates CdmData with basic fields. Kept for test convenience.
func NewLegacyCdmData(tobt, tsat, ttot, ctot, aobt, asat, eobt, status *string) *CdmData {
	return &CdmData{
		Tobt:   tobt,
		Tsat:   tsat,
		Ttot:   ttot,
		Ctot:   ctot,
		Aobt:   aobt,
		Asat:   asat,
		Eobt:   eobt,
		Status: status,
	}
}

func (d *CdmData) Clone() *CdmData {
	if d == nil {
		return &CdmData{}
	}
	clone := *d
	clone.Tobt = cloneStringPointer(d.Tobt)
	clone.TobtSetBy = cloneStringPointer(d.TobtSetBy)
	clone.TobtConfirmedBy = cloneStringPointer(d.TobtConfirmedBy)
	clone.ReqTobt = cloneStringPointer(d.ReqTobt)
	clone.ReqTobtType = cloneStringPointer(d.ReqTobtType)
	clone.Tsat = cloneStringPointer(d.Tsat)
	clone.Ttot = cloneStringPointer(d.Ttot)
	clone.Ctot = cloneStringPointer(d.Ctot)
	clone.CtotSource = cloneStringPointer(d.CtotSource)
	clone.Aobt = cloneStringPointer(d.Aobt)
	clone.Asat = cloneStringPointer(d.Asat)
	clone.Asrt = cloneStringPointer(d.Asrt)
	clone.Tsac = cloneStringPointer(d.Tsac)
	clone.Eobt = cloneStringPointer(d.Eobt)
	clone.Aldt = cloneStringPointer(d.Aldt)
	clone.Status = cloneStringPointer(d.Status)
	clone.DeIce = cloneStringPointer(d.DeIce)
	clone.EcfmpID = cloneStringPointer(d.EcfmpID)
	clone.Phase = cloneStringPointer(d.Phase)
	clone.Calculation = d.Calculation.Clone()
	return &clone
}

func (c *CdmCalculation) Clone() *CdmCalculation {
	if c == nil {
		return nil
	}
	clone := *c
	clone.BaseTime = cloneStringPointer(c.BaseTime)
	clone.BaseSource = cloneStringPointer(c.BaseSource)
	clone.TaxiMinutes = cloneIntPointer(c.TaxiMinutes)
	clone.TaxiRunway = cloneStringPointer(c.TaxiRunway)
	clone.InvalidReason = cloneStringPointer(c.InvalidReason)
	return &clone
}

// Normalize trims empty nested calculation snapshots from persisted state.
func (d *CdmData) Normalize() *CdmData {
	if d == nil {
		return &CdmData{}
	}

	if calculationIsEmpty(d.Calculation) {
		d.Calculation = nil
	}

	return d
}

func (d *CdmData) EffectiveTobt() *string {
	if d == nil {
		return nil
	}
	return d.Tobt
}

func (d *CdmData) EffectiveReqTobt() *string {
	if d == nil {
		return nil
	}
	return d.ReqTobt
}

func (d *CdmData) EffectiveReqTobtType() *string {
	if d == nil {
		return nil
	}
	return d.ReqTobtType
}

func (d *CdmData) EffectiveTsat() *string {
	if d == nil {
		return nil
	}
	return d.Tsat
}

func (d *CdmData) EffectiveTtot() *string {
	if d == nil {
		return nil
	}
	return d.Ttot
}

func (d *CdmData) EffectiveCtot() *string {
	if d == nil {
		return nil
	}
	return d.Ctot
}

func (d *CdmData) EffectiveAobt() *string {
	if d == nil {
		return nil
	}
	return d.Aobt
}

func (d *CdmData) EffectiveAsat() *string {
	if d == nil {
		return nil
	}
	return d.Asat
}

func (d *CdmData) EffectiveEobt() *string {
	if d == nil {
		return nil
	}
	return d.Eobt
}

func (d *CdmData) EffectiveStatus() *string {
	if d == nil {
		return nil
	}
	return d.Status
}

func (d *CdmData) EffectiveAldt() *string {
	if d == nil {
		return nil
	}
	return d.Aldt
}

func (d *CdmData) NeedsLocalRecalculation() bool {
	return d != nil && d.Recalculate
}

func (d *CdmData) MarkLocalRecalculationPending() {
	if d == nil {
		return
	}
	d.Recalculate = true
}

func (d *CdmData) ClearLocalRecalculationPending() {
	if d == nil {
		return
	}
	d.Recalculate = false
}

func (d *CdmData) HasManualCtot() bool {
	if d == nil {
		return false
	}
	return d.CtotSource != nil && *d.CtotSource == CtotSourceManual && d.Ctot != nil && *d.Ctot != ""
}

func (d *CdmData) EffectivePhase() *string {
	if d == nil {
		return nil
	}
	return d.Phase
}

func (d *CdmData) EffectiveCalculation() *CdmCalculation {
	if d == nil {
		return nil
	}
	return d.Normalize().Calculation
}

func (d *CdmData) EffectiveTaxiMinutesForRunway(runway string) *int {
	if d == nil {
		return nil
	}

	calculation := d.EffectiveCalculation()
	if calculation == nil || calculation.TaxiMinutes == nil {
		return nil
	}

	storedRunway := strings.TrimSpace(stringValue(calculation.TaxiRunway))
	if storedRunway == "" || runway == "" || strings.EqualFold(storedRunway, strings.TrimSpace(runway)) {
		return calculation.TaxiMinutes
	}

	return nil
}

func stringPointer(value string) *string {
	return &value
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func calculationIsEmpty(calculation *CdmCalculation) bool {
	return calculation == nil ||
		(calculation.BaseTime == nil &&
			calculation.BaseSource == nil &&
			calculation.TaxiMinutes == nil &&
			calculation.TaxiRunway == nil &&
			calculation.InvalidReason == nil)
}
