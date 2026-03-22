package models

import "time"

type CdmFieldOverride struct {
	Value          string     `json:"value"`
	ObservedAt     time.Time  `json:"observedAt"`
	SourcePosition string     `json:"sourcePosition"`
	SourceRole     string     `json:"sourceRole"`
	ExpiresAt      *time.Time `json:"expiresAt,omitempty"`
}

type CdmCanonical struct {
	Tobt      *string    `json:"tobt,omitempty"`
	Tsat      *string    `json:"tsat,omitempty"`
	Ttot      *string    `json:"ttot,omitempty"`
	Ctot      *string    `json:"ctot,omitempty"`
	Aobt      *string    `json:"aobt,omitempty"`
	Asat      *string    `json:"asat,omitempty"`
	Eobt      *string    `json:"eobt,omitempty"`
	Status    *string    `json:"status,omitempty"`
	Source    string     `json:"source,omitempty"`
	UpdatedAt *time.Time `json:"updatedAt,omitempty"`
}

type CdmPluginData struct {
	Asrt       *string `json:"asrt,omitempty"`
	Tsac       *string `json:"tsac,omitempty"`
	DeIce      *string `json:"deIce,omitempty"`
	EcfmpID    *string `json:"ecfmpId,omitempty"`
	ManualCtot *string `json:"manualCtot,omitempty"`
}

type CdmPendingRequest struct {
	RequestedAt    *time.Time `json:"requestedAt,omitempty"`
	Via            string     `json:"via,omitempty"`
	TargetPosition *string    `json:"targetPosition,omitempty"`
}

type CdmData struct {
	Canonical      CdmCanonical                `json:"canonical"`
	LocalOverrides map[string]CdmFieldOverride `json:"localOverrides,omitempty"`
	Plugin         CdmPluginData               `json:"plugin,omitempty"`
	Pending        *CdmPendingRequest          `json:"pending,omitempty"`
}

type CdmDataRow struct {
	Callsign string
	Data     *CdmData
}

func NewLegacyCdmData(tobt, tsat, ttot, ctot, aobt, asat, eobt, status *string) *CdmData {
	return (&CdmData{
		Canonical: CdmCanonical{
			Tobt:   tobt,
			Tsat:   tsat,
			Ttot:   ttot,
			Ctot:   ctot,
			Aobt:   aobt,
			Asat:   asat,
			Eobt:   eobt,
			Status: status,
		},
	}).Normalize()
}

func (d *CdmData) Clone() *CdmData {
	if d == nil {
		return (&CdmData{}).Normalize()
	}

	clone := *d
	if d.Pending != nil {
		pending := *d.Pending
		clone.Pending = &pending
	}
	if d.LocalOverrides != nil {
		clone.LocalOverrides = make(map[string]CdmFieldOverride, len(d.LocalOverrides))
		for key, value := range d.LocalOverrides {
			clone.LocalOverrides[key] = value
		}
	}

	return (&clone).Normalize()
}

func (d *CdmData) Normalize() *CdmData {
	if d == nil {
		return &CdmData{}
	}
	if len(d.LocalOverrides) == 0 {
		d.LocalOverrides = nil
	}
	return d
}

func (d *CdmData) EffectiveTobt() *string {
	return d.effectiveValue("tobt", d.Canonical.Tobt)
}

func (d *CdmData) EffectiveTsat() *string {
	return d.effectiveValue("tsat", d.Canonical.Tsat)
}

func (d *CdmData) EffectiveTtot() *string {
	return d.effectiveValue("ttot", d.Canonical.Ttot)
}

func (d *CdmData) EffectiveCtot() *string {
	return d.effectiveValue("ctot", d.Canonical.Ctot)
}

func (d *CdmData) EffectiveAobt() *string {
	return d.effectiveValue("aobt", d.Canonical.Aobt)
}

func (d *CdmData) EffectiveAsat() *string {
	return d.effectiveValue("asat", d.Canonical.Asat)
}

func (d *CdmData) EffectiveEobt() *string {
	return d.effectiveValue("eobt", d.Canonical.Eobt)
}

func (d *CdmData) EffectiveStatus() *string {
	if d == nil {
		return nil
	}
	return d.Canonical.Status
}

func (d *CdmData) ClearMatchingLocalOverride(field string, canonical *string) {
	if d == nil || len(d.LocalOverrides) == 0 || canonical == nil {
		return
	}

	override, ok := d.LocalOverrides[field]
	if !ok || override.Value != *canonical {
		return
	}

	delete(d.LocalOverrides, field)
	if len(d.LocalOverrides) == 0 {
		d.LocalOverrides = nil
	}
}

func (d *CdmData) effectiveValue(field string, canonical *string) *string {
	if d != nil {
		if override, ok := d.LocalOverrides[field]; ok && override.Value != "" {
			return stringPointer(override.Value)
		}
	}

	return canonical
}

func stringPointer(value string) *string {
	return &value
}
