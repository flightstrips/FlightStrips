package models

const (
	CtotSourceManual = "Manual"
	CtotSourceEvent  = "EVENT"
	CtotSourceATFCM  = "ATFCM"

	TobtConfirmedByATC   = "ATC"
	TobtConfirmedByPilot = "Pilot"
)

type CdmData struct {
	Tobt            *string `json:"tobt,omitempty"`
	TobtSetBy       *string `json:"tobtSetBy,omitempty"`
	TobtConfirmedBy *string `json:"tobtConfirmedBy,omitempty"`
	ReqTobt     *string `json:"reqTobt,omitempty"`
	Tsat        *string `json:"tsat,omitempty"`
	Ttot        *string `json:"ttot,omitempty"`
	Ctot        *string `json:"ctot,omitempty"`
	CtotSource  *string `json:"ctotSource,omitempty"`
	Aobt        *string `json:"aobt,omitempty"`
	Asat        *string `json:"asat,omitempty"`
	Asrt        *string `json:"asrt,omitempty"`
	Tsac        *string `json:"tsac,omitempty"`
	Eobt        *string `json:"eobt,omitempty"`
	Status      *string `json:"status,omitempty"`
	DeIce       *string `json:"deIce,omitempty"`
	EcfmpID     *string `json:"ecfmpId,omitempty"`
	Phase       *string `json:"phase,omitempty"`
	Recalculate bool    `json:"recalculate,omitempty"`
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
	return &clone
}

// Normalize is a no-op on the flat struct, kept for call-site compatibility.
func (d *CdmData) Normalize() *CdmData {
	if d == nil {
		return &CdmData{}
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

func stringPointer(value string) *string {
	return &value
}
