package shared

import (
	"FlightStrips/internal/models"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"encoding/json"
)

func BuildEuroscopeCdmUpdateEvent(callsign string, data *models.CdmData) euroscopeEvents.CdmUpdateEvent {
	if data == nil {
		data = (&models.CdmData{}).Normalize()
	}

	return euroscopeEvents.CdmUpdateEvent{
		Callsign:              callsign,
		Eobt:                  truncateCDMClockValue(valueOrEmpty(data.EffectiveEobt())),
		Tobt:                  truncateCDMClockValue(valueOrEmpty(data.EffectiveTobt())),
		TobtSetBy:             valueOrEmpty(data.TobtSetBy),
		TobtConfirmedBy:       valueOrEmpty(data.TobtConfirmedBy),
		ReqTobt:               truncateCDMClockValue(valueOrEmpty(data.EffectiveReqTobt())),
		ReqTobtType:           valueOrEmpty(data.EffectiveReqTobtType()),
		Tsat:                  truncateCDMClockValue(valueOrEmpty(data.EffectiveTsat())),
		Ttot:                  truncateCDMClockValue(valueOrEmpty(data.EffectiveTtot())),
		Ctot:                  truncateCDMClockValue(valueOrEmpty(data.EffectiveCtot())),
		CtotSource:            valueOrEmpty(data.CtotSource),
		Asat:                  truncateCDMClockValue(valueOrEmpty(data.EffectiveAsat())),
		Asrt:                  truncateCDMClockValue(valueOrEmpty(data.Asrt)),
		Tsac:                  valueOrEmpty(data.Tsac),
		Status:                valueOrEmpty(data.EffectiveStatus()),
		EcfmpID:               valueOrEmpty(data.EcfmpID),
		Phase:                 valueOrEmpty(data.EffectivePhase()),
		EcfmpRestrictionsJSON: serializeEcfmpRestrictionsJSON(data.EcfmpRestrictions),
	}
}

func BuildEuroscopeBackendSyncCdmData(data *models.CdmData) euroscopeEvents.BackendSyncCdmData {
	update := BuildEuroscopeCdmUpdateEvent("", data)
	return euroscopeEvents.BackendSyncCdmData{
		Eobt:                  update.Eobt,
		Tobt:                  update.Tobt,
		TobtSetBy:             update.TobtSetBy,
		TobtConfirmedBy:       update.TobtConfirmedBy,
		ReqTobt:               update.ReqTobt,
		ReqTobtType:           update.ReqTobtType,
		Tsat:                  update.Tsat,
		Ttot:                  update.Ttot,
		Ctot:                  update.Ctot,
		CtotSource:            update.CtotSource,
		Asat:                  update.Asat,
		Asrt:                  update.Asrt,
		Tsac:                  update.Tsac,
		Status:                update.Status,
		EcfmpID:               update.EcfmpID,
		Phase:                 update.Phase,
		EcfmpRestrictionsJSON: update.EcfmpRestrictionsJSON,
	}
}

func serializeEcfmpRestrictionsJSON(restrictions []models.EcfmpRestriction) string {
	if len(restrictions) == 0 {
		return ""
	}

	dtos := convertEcfmpRestrictionsEuroscope(restrictions)
	b, err := json.Marshal(dtos)
	if err != nil {
		return ""
	}
	return string(b)
}

func convertEcfmpRestrictionsEuroscope(restrictions []models.EcfmpRestriction) []euroscopeEvents.EcfmpRestrictionDTO {
	if len(restrictions) == 0 {
		return nil
	}

	result := make([]euroscopeEvents.EcfmpRestrictionDTO, len(restrictions))
	for i, r := range restrictions {
		result[i] = euroscopeEvents.EcfmpRestrictionDTO{
			MeasureID:   r.MeasureID,
			Ident:       r.Ident,
			Type:        r.Type,
			Reason:      r.Reason,
			Routes:      r.Routes,
			Destination: r.Destination,
			MaxLevel:    r.MaxLevel,
			MinLevel:    r.MinLevel,
			ExactLevels: r.ExactLevels,
			HasCtot:     r.HasCtot,
		}
	}
	return result
}

func truncateCDMClockValue(value string) string {
	if len(value) > 4 {
		return value[:4]
	}
	return value
}
