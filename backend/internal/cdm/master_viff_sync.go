package cdm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/helpers"

	"github.com/jackc/pgx/v5"
)

type MasterViffSync struct {
	service *Service
}

type viffPushState struct {
	Suspend bool
	Params  SetCdmDataParams
}

func (c *MasterViffSync) ensureMasterFlightExport(ctx context.Context, session int32, callsign string, local *models.CdmData, remote IFPSData) {
	s := c.service
	if !s.client.isValid || !s.usesViffSession(session) || local == nil || local.NeedsLocalRecalculation() || !masterFlightNeedsExport(local, remote) {
		return
	}

	strip, err := s.stripRepo.GetByCallsign(ctx, session, callsign)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.WarnContext(ctx, "Failed to load strip for master CDM export",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.Any("error", err),
		)
	}

	c.pushViffAfterRecalcAsync(ctx, session, callsign, strip, local)
}

func (c *MasterViffSync) mergeMasterViffFlight(ctx context.Context, session int32, callsign string, flight *models.CdmData, row IFPSData, nextCtot string, nextCtotSource string) (*models.CdmData, bool, error) {
	s := c.service
	if flight == nil {
		return nil, false, nil
	}

	ctotChanged := helpers.ValueOrDefault(flight.Ctot) != nextCtot
	requestedTobt := truncateCDMClockValue(strings.TrimSpace(row.CDMData.ReqTOBT))
	requestSource := strings.ToUpper(strings.TrimSpace(row.CDMData.ReqTOBTType))
	if requestSource == "" {
		requestSource = "VIFF"
	}
	requestChanged := requestedTobt != "" && isValidHHMM(requestedTobt) &&
		(helpers.ValueOrDefault(flight.Tobt) != requestedTobt ||
			helpers.ValueOrDefault(flight.TobtSetBy) != "vIFF" ||
			helpers.ValueOrDefault(flight.TobtConfirmedBy) != requestSource)
	changed := ctotChanged || requestChanged ||
		helpers.ValueOrDefault(flight.MostPenalizingAirspace) != row.MostPenalizingAirspace ||
		helpers.ValueOrDefault(flight.EcfmpID) != row.CDMData.Reason
	if !changed {
		return flight, false, nil
	}

	before := snapshotCdm(flight)
	updated := flight.Clone()
	if nextCtot != "" {
		updated.Ctot = &nextCtot
		updated.CtotSource = &nextCtotSource
		updated.MostPenalizingAirspace = stringPointerIfPresent(row.MostPenalizingAirspace)
		updated.EcfmpID = stringPointerIfPresent(row.CDMData.Reason)
	} else if !flight.HasManualCtot() {
		updated.Ctot = nil
		updated.CtotSource = nil
		updated.MostPenalizingAirspace = nil
		updated.EcfmpID = nil
	}
	needsRecalculate := ctotChanged
	if requestChanged {
		updated.Tobt = &requestedTobt
		setBy := "vIFF"
		updated.TobtSetBy = &setBy
		updated.TobtConfirmedBy = &requestSource
		updated.TobtAutoSynced = false
		updated.TobtManuallyConfirmed = true
		updated.ViffRequestSyncPending = true
		needsRecalculate = applyTobtRecalculationPolicy(updated, requestedTobt) || needsRecalculate
	} else if ctotChanged {
		updated.MarkLocalRecalculationPending()
	}

	if err := s.persistCdmUpdate(ctx, session, callsign, before, updated); err != nil {
		return nil, false, err
	}
	s.reevaluateCtotValidationAsync(ctx, session, callsign, before, snapshotCdm(updated))
	return updated, needsRecalculate, nil
}

func (c *MasterViffSync) pushViffDataAfterRecalc(ctx context.Context, session int32, callsign string) {
	s := c.service
	if !s.client.isValid || !s.usesViffSession(session) {
		return
	}

	data, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		slog.WarnContext(ctx, "Failed to load recalculated CDM data",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.Any("error", err),
		)
		return
	}
	// READY-derived state is exported synchronously by completeReadyViffSync
	// only after this flight's own REA/1 has succeeded. An airport-wide
	// recalculation for another flight must not bypass that ordering.
	if data.ReadySyncPending {
		return
	}

	// Load strip for departure info (runway) needed by setCdmData
	strip, _ := s.stripRepo.GetByCallsign(ctx, session, callsign)
	c.pushViffAfterRecalcAsync(ctx, session, callsign, strip, data)
}

func (c *MasterViffSync) pushCdmDataAfterRecalc(ctx context.Context, session int32, callsign string) {
	s := c.service
	data, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		slog.WarnContext(ctx, "Failed to load recalculated CDM data",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.Any("error", err),
		)
		return
	}

	s.publisher.SendCdmUpdates(session, []frontendEvents.CdmDataEvent{shared.BuildFrontendCdmDataEvent(callsign, data)})
	s.euroscopeHub.BroadcastCdmUpdates(session, []euroscopeEvents.CdmUpdateEvent{shared.BuildEuroscopeCdmUpdateEvent(callsign, data)})

	if !s.client.isValid || !s.usesViffSession(session) {
		return
	}

	strip, _ := s.stripRepo.GetByCallsign(ctx, session, callsign)
	c.pushViffAfterRecalcAsync(ctx, session, callsign, strip, data)
}

func (c *MasterViffSync) pushViffAfterRecalcAsync(ctx context.Context, session int32, callsign string, strip *models.Strip, data *models.CdmData) {
	s := c.service
	if !s.client.isValid || !s.usesViffSession(session) {
		return
	}
	state, ok := buildViffPushState(callsign, strip, data)
	if !ok || !s.markViffPushPending(session, callsign, state) {
		return
	}
	asyncCtx := detachedContext(ctx)
	go func() {
		if err := s.pushViffState(asyncCtx, callsign, state); err != nil {
			s.clearPendingViffPush(session, callsign, state)
			slog.WarnContext(asyncCtx, "Failed to push CDM data to CDM backend",
				slog.String("callsign", callsign),
				slog.Any("error", err),
			)
		}
	}()
}

func (c *MasterViffSync) pushViffAfterRecalc(ctx context.Context, callsign string, strip *models.Strip, data *models.CdmData) error {
	s := c.service
	state, ok := buildViffPushState(callsign, strip, data)
	if !ok {
		return nil
	}
	return s.pushViffState(ctx, callsign, state)
}

func (c *MasterViffSync) pushAuthoritativeViffState(ctx context.Context, callsign string, strip *models.Strip, data *models.CdmData) error {
	if _, ok := buildViffPushState(callsign, strip, data); ok {
		return c.pushViffAfterRecalc(ctx, callsign, strip, data)
	}
	tobt := truncateCDMClockValue(helpers.ValueOrDefault(data.EffectiveTobt()))
	if !isValidHHMM(tobt) {
		return fmt.Errorf("cannot export authoritative CDM state for %s without a valid TOBT", callsign)
	}
	return c.service.client.IFPSSetTobt(ctx, callsign, tobt, c.service.resolveTaxiMinutes(strip))
}

func (c *MasterViffSync) pushViffState(ctx context.Context, callsign string, state viffPushState) error {
	s := c.service
	if state.Suspend {
		return s.client.IFPSDpi(ctx, callsign, "SUSP")
	}
	return s.client.IFPSSetCdmData(ctx, state.Params)
}

func (c *MasterViffSync) markViffPushPending(session int32, callsign string, state viffPushState) bool {
	s := c.service
	key := viffPushKey(session, callsign)
	current, ok := s.lastPushedViff.Load(key)
	if ok && current.(viffPushState) == state {
		return false
	}
	s.lastPushedViff.Store(key, state)
	return true
}

func (c *MasterViffSync) clearPendingViffPush(session int32, callsign string, state viffPushState) {
	s := c.service
	key := viffPushKey(session, callsign)
	current, ok := s.lastPushedViff.Load(key)
	if ok && current.(viffPushState) == state {
		s.lastPushedViff.Delete(key)
	}
}

func (c *MasterViffSync) pushLatestMasterCdmDataToViff(ctx context.Context, session int32, callsign string, strip *models.Strip) error {
	s := c.service
	data, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}
	return s.pushViffAfterRecalc(ctx, callsign, strip, data)
}

func (c *MasterViffSync) refreshMasterFlightFromViff(ctx context.Context, session int32, callsign string, airport string) error {
	s := c.service
	payload, err := s.client.IFPSByCallsign(ctx, callsign)
	if err != nil {
		return err
	}

	row, err := parseIFPSByCallsignResponse(payload)
	if err != nil || row == nil {
		return err
	}

	flight, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	nextCtot, nextCtotSource := effectiveIfpsCtotAndSource(*row)
	_, needsRecalculate, err := s.mergeMasterViffFlight(ctx, session, callsign, flight, *row, nextCtot, nextCtotSource)
	if err != nil {
		return err
	}
	if !needsRecalculate || s.sequenceService == nil || airport == "" || !s.canRunLocalRecalculation(session) {
		return nil
	}

	if err := s.sequenceService.RecalculateAirportSilently(ctx, session, airport); err != nil {
		return err
	}
	s.pushCdmDataAfterRecalc(ctx, session, callsign)
	return nil
}

func (c *MasterViffSync) masterPosition() string {
	return DefaultMasterPosition
}

func (c *MasterViffSync) registerMasterAsync(ctx context.Context, airport string) {
	s := c.service
	if !s.client.isValid || airport == "" {
		return
	}
	position := s.masterPosition()
	asyncCtx := detachedContext(ctx)
	go func() {
		if err := s.client.SetMasterAirport(asyncCtx, airport, position); err != nil {
			slog.Warn("Failed to register CDM master airport",
				slog.String("airport", airport),
				slog.String("position", position),
				slog.Any("error", err),
			)
		}
	}()
}

func (c *MasterViffSync) deregisterMaster(ctx context.Context, airport string) error {
	s := c.service
	airport = strings.TrimSpace(airport)
	if !s.client.isValid || airport == "" {
		return nil
	}
	return s.client.ClearMasterAirport(ctx, airport, s.masterPosition())
}

func masterFlightNeedsExport(local *models.CdmData, remote IFPSData) bool {
	if local == nil {
		return false
	}

	if helpers.ValueOrDefault(local.EffectivePhase()) == "I" {
		return !statusImpliesInvalidation(remote.CDMStatus)
	}

	localTsat := truncateCDMClockValue(helpers.ValueOrDefault(local.EffectiveTsat()))
	if localTsat == "" {
		return false
	}

	localTobt := truncateCDMClockValue(helpers.ValueOrDefault(local.EffectiveTobt()))
	localTtot := truncateCDMClockValue(helpers.ValueOrDefault(local.EffectiveTtot()))
	localCtot := truncateCDMClockValue(helpers.ValueOrDefault(local.EffectiveCtot()))
	localAsrt := truncateCDMClockValue(helpers.ValueOrDefault(local.Asrt))
	localReason := helpers.ValueOrDefault(local.EcfmpID)

	remoteTobt := truncateCDMClockValue(remote.TOBT)
	remoteTsat := truncateCDMClockValue(remote.CDMData.TSAT)
	remoteTtot := truncateCDMClockValue(remote.CDMData.TTOT)
	remoteCtot, _ := effectiveIfpsCtotAndSource(remote)
	remoteAsrt := truncateCDMClockValue(remote.CDMData.ReqASRT)
	remoteReason := remote.CDMData.Reason

	return localTobt != remoteTobt ||
		localTsat != remoteTsat ||
		localTtot != remoteTtot ||
		localCtot != remoteCtot ||
		localAsrt != remoteAsrt ||
		localReason != remoteReason
}

func buildViffPushState(callsign string, strip *models.Strip, data *models.CdmData) (viffPushState, bool) {
	if data == nil {
		return viffPushState{}, false
	}
	if helpers.ValueOrDefault(data.EffectivePhase()) == "I" {
		return viffPushState{Suspend: true}, true
	}

	tsat := normalizeViffCdmTime(helpers.ValueOrDefault(data.EffectiveTsat()))
	if tsat == "" {
		return viffPushState{}, false
	}

	depInfo := ""
	if strip != nil && strip.Runway != nil {
		depInfo = *strip.Runway
	}

	return viffPushState{
		Params: SetCdmDataParams{
			Callsign: callsign,
			Tobt:     normalizeViffCdmTime(helpers.ValueOrDefault(data.EffectiveTobt())),
			Tsat:     tsat,
			Ttot:     normalizeViffCdmTime(helpers.ValueOrDefault(data.EffectiveTtot())),
			Ctot:     truncateCDMClockValue(helpers.ValueOrDefault(data.EffectiveCtot())),
			Reason:   helpers.ValueOrDefault(data.EcfmpID),
			Asrt:     normalizeViffCdmTime(helpers.ValueOrDefault(data.Asrt)),
			DepInfo:  depInfo,
		},
	}, true
}

func viffPushKey(session int32, callsign string) string {
	return strconv.Itoa(int(session)) + ":" + callsign
}

func parseIFPSByCallsignResponse(payload []byte) (*IFPSData, error) {
	trimmed := strings.TrimSpace(string(payload))
	if trimmed == "" || trimmed == "null" || trimmed == "true" || trimmed == "false" {
		return nil, nil
	}

	var single IFPSData
	if err := json.Unmarshal(payload, &single); err == nil {
		if strings.TrimSpace(single.Callsign) == "" {
			return nil, nil
		}
		return &single, nil
	}

	var many []IFPSData
	if err := json.Unmarshal(payload, &many); err != nil {
		return nil, err
	}
	for _, row := range many {
		if strings.TrimSpace(row.Callsign) != "" {
			result := row
			return &result, nil
		}
	}
	return nil, nil
}

func normalizeViffCdmTime(value string) string {
	value = normalizeCalculationClock(value)
	if value == "" {
		return ""
	}
	return toHHMMSS(value)
}
