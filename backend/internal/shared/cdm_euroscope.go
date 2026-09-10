package shared

import (
	"FlightStrips/internal/models"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
)

func BuildEuroscopeCdmUpdateEvent(callsign string, data *models.CdmData) euroscopeEvents.CdmUpdateEvent {
	if data == nil {
		data = (&models.CdmData{}).Normalize()
	}

	return euroscopeEvents.CdmUpdateEvent{
		Callsign:          callsign,
		Eobt:              truncateCDMClockValue(valueOrEmpty(data.EffectiveEobt())),
		Tobt:              truncateCDMClockValue(valueOrEmpty(data.EffectiveTobt())),
		TobtSetBy:         valueOrEmpty(data.TobtSetBy),
		TobtConfirmedBy:   valueOrEmpty(data.TobtConfirmedBy),
		Tsat:              truncateCDMClockValue(valueOrEmpty(data.EffectiveTsat())),
		Ttot:              truncateCDMClockValue(valueOrEmpty(data.EffectiveTtot())),
		Ctot:              truncateCDMClockValue(valueOrEmpty(data.EffectiveCtot())),
		CtotSource:        valueOrEmpty(data.CtotSource),
		Asat:              truncateCDMClockValue(valueOrEmpty(data.EffectiveAsat())),
		Asrt:              truncateCDMClockValue(valueOrEmpty(data.Asrt)),
		Tsac:              valueOrEmpty(data.Tsac),
		Status:            valueOrEmpty(data.EffectiveStatus()),
		EcfmpId:           valueOrEmpty(data.EcfmpID),
		Phase:             valueOrEmpty(data.EffectivePhase()),
		EcfmpRestrictions: convertEcfmpRestrictionsEuroscope(data.EcfmpRestrictions),
	}
}

func BuildEuroscopeBackendSyncCdmData(data *models.CdmData) euroscopeEvents.BackendSyncCdmData {
	update := BuildEuroscopeCdmUpdateEvent("", data)
	return euroscopeEvents.BackendSyncCdmData{
		Eobt:              update.Eobt,
		Tobt:              update.Tobt,
		TobtSetBy:         update.TobtSetBy,
		TobtConfirmedBy:   update.TobtConfirmedBy,
		Tsat:              update.Tsat,
		Ttot:              update.Ttot,
		Ctot:              update.Ctot,
		CtotSource:        update.CtotSource,
		Asat:              update.Asat,
		Asrt:              update.Asrt,
		Tsac:              update.Tsac,
		Status:            update.Status,
		EcfmpId:           update.EcfmpId,
		Phase:             update.Phase,
		EcfmpRestrictions: update.EcfmpRestrictions,
	}
}

func convertEcfmpRestrictionsEuroscope(restrictions []models.EcfmpRestriction) []*euroscopeEvents.EcfmpRestriction {
	if len(restrictions) == 0 {
		return nil
	}

	result := make([]*euroscopeEvents.EcfmpRestriction, len(restrictions))
	for i, r := range restrictions {
		result[i] = &euroscopeEvents.EcfmpRestriction{
			MeasureId:   r.MeasureID,
			Ident:       r.Ident,
			Type:        r.Type,
			Reason:      r.Reason,
			Routes:      r.Routes,
			Destination: r.Destination,
			MaxLevel:    intToInt32(r.MaxLevel),
			MinLevel:    intToInt32(r.MinLevel),
			ExactLevels: intsToInt32s(r.ExactLevels),
			HasCtot:     r.HasCtot,
		}
	}
	return result
}

func intToInt32(value *int) *int32 {
	if value == nil {
		return nil
	}
	converted := int32(*value)
	return &converted
}

func intsToInt32s(values []int) []int32 {
	result := make([]int32, len(values))
	for i, value := range values {
		result[i] = int32(value)
	}
	return result
}

func truncateCDMClockValue(value string) string {
	if len(value) > 4 {
		return value[:4]
	}
	return value
}
