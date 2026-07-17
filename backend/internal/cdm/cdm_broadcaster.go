package cdm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/helpers"
)

type CdmBroadcaster struct {
	service *Service
}

type cdmSnapshot struct {
	Eobt, Tobt, Tsat, Ctot, CtotSource, Ttot, Asat, Asrt, Tsac, Aobt, Status, ReqTobt, ReqTobtType, MostPenalizingAirspace, EcfmpID, TobtSetBy, TobtConfirmedBy, Phase string
	EcfmpRestrictionsJSON                                                                                                                                              string
	TobtAutoSynced, TobtManuallyConfirmed                                                                                                                              bool
}

func (c *CdmBroadcaster) broadcastIfChanged(session int32, callsign string, before, after cdmSnapshot) {
	s := c.service
	if before == after {
		return
	}

	cdmData := &models.CdmData{
		Eobt:                   stringPointerIfPresent(after.Eobt),
		Tobt:                   stringPointerIfPresent(after.Tobt),
		ReqTobt:                stringPointerIfPresent(after.ReqTobt),
		ReqTobtType:            stringPointerIfPresent(after.ReqTobtType),
		Tsat:                   stringPointerIfPresent(after.Tsat),
		Ttot:                   stringPointerIfPresent(after.Ttot),
		Ctot:                   stringPointerIfPresent(after.Ctot),
		CtotSource:             stringPointerIfPresent(after.CtotSource),
		Aobt:                   stringPointerIfPresent(after.Aobt),
		Asat:                   stringPointerIfPresent(after.Asat),
		Asrt:                   stringPointerIfPresent(after.Asrt),
		Tsac:                   stringPointerIfPresent(after.Tsac),
		Status:                 stringPointerIfPresent(after.Status),
		MostPenalizingAirspace: stringPointerIfPresent(after.MostPenalizingAirspace),
		EcfmpID:                stringPointerIfPresent(after.EcfmpID),
		Phase:                  stringPointerIfPresent(after.Phase),
	}
	if before.EcfmpRestrictionsJSON != after.EcfmpRestrictionsJSON {
		storedData, err := s.stripRepo.GetCdmDataForCallsign(context.Background(), session, callsign)
		if err == nil && storedData != nil {
			cdmData.EcfmpRestrictions = storedData.EcfmpRestrictions
		}
	}
	s.publisher.SendCdmUpdates(session, []frontendEvents.CdmDataEvent{shared.BuildFrontendCdmDataEvent(callsign, cdmData)})

	data, err := s.stripRepo.GetCdmDataForCallsign(context.Background(), session, callsign)
	if err == nil {
		s.euroscopeHub.BroadcastCdmUpdates(session, []euroscopeEvents.CdmUpdateEvent{shared.BuildEuroscopeCdmUpdateEvent(callsign, data)})
	}
}

func (c *CdmBroadcaster) persistCdmUpdate(ctx context.Context, session int32, callsign string, before cdmSnapshot, updated *models.CdmData) error {
	s := c.service
	rows, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated.Normalize())
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("failed to persist CDM data for %s session %d", callsign, session)
	}
	s.broadcastIfChanged(session, callsign, before, snapshotCdm(updated))
	return nil
}

func (c *CdmBroadcaster) persistCdmUpdateSilently(ctx context.Context, session int32, callsign string, updated *models.CdmData) error {
	s := c.service
	rows, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated.Normalize())
	if err != nil {
		return err
	}
	if rows != 1 {
		return fmt.Errorf("failed to persist CDM data for %s session %d", callsign, session)
	}
	return nil
}

func snapshotCdm(data *models.CdmData) cdmSnapshot {
	if data == nil {
		return cdmSnapshot{}
	}
	ecfmpJSON, _ := json.Marshal(data.EcfmpRestrictions)
	return cdmSnapshot{
		Eobt:                   truncateCDMClockValue(helpers.ValueOrDefault(data.Eobt)),
		Tobt:                   truncateCDMClockValue(helpers.ValueOrDefault(data.Tobt)),
		Tsat:                   truncateCDMClockValue(helpers.ValueOrDefault(data.Tsat)),
		Ctot:                   truncateCDMClockValue(helpers.ValueOrDefault(data.Ctot)),
		CtotSource:             helpers.ValueOrDefault(data.CtotSource),
		Ttot:                   truncateCDMClockValue(helpers.ValueOrDefault(data.Ttot)),
		Asat:                   truncateCDMClockValue(helpers.ValueOrDefault(data.Asat)),
		Asrt:                   truncateCDMClockValue(helpers.ValueOrDefault(data.Asrt)),
		Tsac:                   helpers.ValueOrDefault(data.Tsac),
		Aobt:                   truncateCDMClockValue(helpers.ValueOrDefault(data.Aobt)),
		Status:                 helpers.ValueOrDefault(data.Status),
		ReqTobt:                truncateCDMClockValue(helpers.ValueOrDefault(data.ReqTobt)),
		ReqTobtType:            helpers.ValueOrDefault(data.ReqTobtType),
		MostPenalizingAirspace: helpers.ValueOrDefault(data.MostPenalizingAirspace),
		EcfmpID:                helpers.ValueOrDefault(data.EcfmpID),
		TobtSetBy:              helpers.ValueOrDefault(data.TobtSetBy),
		TobtConfirmedBy:        helpers.ValueOrDefault(data.TobtConfirmedBy),
		Phase:                  helpers.ValueOrDefault(data.Phase),
		EcfmpRestrictionsJSON:  string(ecfmpJSON),
		TobtAutoSynced:         data.TobtAutoSynced,
		TobtManuallyConfirmed:  data.TobtManuallyConfirmed,
	}
}

func stringPointerIfPresent(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	result := value
	return &result
}
