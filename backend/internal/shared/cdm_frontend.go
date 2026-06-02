package shared

import (
	"FlightStrips/internal/models"
	frontendEvents "FlightStrips/pkg/events/frontend"
)

func BuildFrontendCdmDataEvent(callsign string, data *models.CdmData) frontendEvents.CdmDataEvent {
	if data == nil {
		data = (&models.CdmData{}).Normalize()
	}

	return frontendEvents.CdmDataEvent{
		Callsign:           callsign,
		Eobt:               truncateFrontendClockValue(valueOrEmpty(data.EffectiveEobt())),
		Tobt:               truncateFrontendClockValue(valueOrEmpty(data.EffectiveTobt())),
		ReqTobt:            truncateFrontendClockValue(valueOrEmpty(data.EffectiveReqTobt())),
		ReqTobtType:        valueOrEmpty(data.EffectiveReqTobtType()),
		TobtSetBy:          valueOrEmpty(data.TobtSetBy),
		Tsat:               truncateFrontendClockValue(valueOrEmpty(data.EffectiveTsat())),
		Ttot:               truncateFrontendClockValue(valueOrEmpty(data.EffectiveTtot())),
		Ctot:               truncateFrontendClockValue(valueOrEmpty(data.EffectiveCtot())),
		Aobt:               truncateFrontendClockValue(valueOrEmpty(data.EffectiveAobt())),
		Asat:               truncateFrontendClockValue(valueOrEmpty(data.EffectiveAsat())),
		Asrt:               truncateFrontendClockValue(valueOrEmpty(data.Asrt)),
		Tsac:               truncateFrontendClockValue(valueOrEmpty(data.Tsac)),
		Status:             valueOrEmpty(data.EffectiveStatus()),
		EcfmpID:            valueOrEmpty(data.EcfmpID),
		CtotSource:         valueOrEmpty(data.CtotSource),
		Phase:              valueOrEmpty(data.EffectivePhase()),
		EcfmpRestrictions:  convertEcfmpRestrictions(data.EcfmpRestrictions),
	}
}

func convertEcfmpRestrictions(restrictions []models.EcfmpRestriction) []frontendEvents.EcfmpRestrictionDTO {
	if len(restrictions) == 0 {
		return nil
	}
	result := make([]frontendEvents.EcfmpRestrictionDTO, len(restrictions))
	for i, r := range restrictions {
		result[i] = frontendEvents.EcfmpRestrictionDTO{
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

func truncateFrontendClockValue(value string) string {
	if len(value) > 4 {
		return value[:4]
	}
	return value
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
