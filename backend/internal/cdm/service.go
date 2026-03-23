package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	"FlightStrips/pkg/helpers"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type Service struct {
	client         *Client
	stripRepo      repository.StripRepository
	sessionRepo    repository.SessionRepository
	controllerRepo repository.ControllerRepository
	frontendHub    shared.FrontendHub
	euroscopeHub   shared.EuroscopeHub
}

type canonicalCdmState struct {
	Tobt   string
	Tsat   string
	Ttot   string
	Ctot   string
	Aobt   string
	Eobt   string
	Status string
	Source string
}

type effectiveCdmSnapshot struct {
	Eobt string
	Tobt string
	Tsat string
	Ctot string
}

const localOverrideTTL = 10 * time.Minute

func NewCdmService(client *Client, stripRepo repository.StripRepository, sessionRepo repository.SessionRepository, controllerRepo repository.ControllerRepository) *Service {
	return &Service{
		client:         client,
		stripRepo:      stripRepo,
		sessionRepo:    sessionRepo,
		controllerRepo: controllerRepo,
	}
}

func (s *Service) SetFrontendHub(frontendHub shared.FrontendHub) {
	s.frontendHub = frontendHub
}

func (s *Service) SetEuroscopeHub(euroscopeHub shared.EuroscopeHub) {
	s.euroscopeHub = euroscopeHub
}

func (s *Service) HandleLocalObservation(ctx context.Context, session int32, observation euroscopeEvents.CdmLocalDataEvent) error {
	callsign := strings.TrimSpace(observation.Callsign)
	sourcePosition := strings.TrimSpace(observation.SourcePosition)
	sourceRole := strings.TrimSpace(observation.SourceRole)

	if callsign == "" {
		slog.Warn("Rejected local CDM observation without callsign",
			slog.Int("session", int(session)),
			slog.String("source_position", sourcePosition),
			slog.String("source_role", sourceRole),
		)
		return nil
	}

	cdmData, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			slog.Warn("Rejected local CDM observation for unknown strip",
				slog.Int("session", int(session)),
				slog.String("callsign", callsign),
				slog.String("source_position", sourcePosition),
				slog.String("source_role", sourceRole),
			)
			return nil
		}
		return err
	}

	before := snapshotEffectiveCdm(cdmData)
	updated := cdmData.Clone()
	now := time.Now().UTC()
	expiresAt := now.Add(localOverrideTTL)

	appliedOverrides := make([]string, 0, 4)
	pluginUpdates := make([]string, 0, 3)

	applyLocalOverride(updated, "tobt", strings.TrimSpace(observation.Tobt), sourcePosition, sourceRole, now, expiresAt, &appliedOverrides)
	applyLocalOverride(updated, "tsat", strings.TrimSpace(observation.Tsat), sourcePosition, sourceRole, now, expiresAt, &appliedOverrides)
	applyLocalOverride(updated, "ttot", strings.TrimSpace(observation.Ttot), sourcePosition, sourceRole, now, expiresAt, &appliedOverrides)
	applyLocalOverride(updated, "ctot", strings.TrimSpace(observation.Ctot), sourcePosition, sourceRole, now, expiresAt, &appliedOverrides)

	setPluginField(&updated.Plugin.Asrt, strings.TrimSpace(observation.Asrt), "asrt", &pluginUpdates)
	setPluginField(&updated.Plugin.Tsac, strings.TrimSpace(observation.Tsac), "tsac", &pluginUpdates)
	setPluginField(&updated.Plugin.ManualCtot, strings.TrimSpace(observation.ManualCtot), "manual_ctot", &pluginUpdates)

	if len(appliedOverrides) == 0 && len(pluginUpdates) == 0 {
		slog.Debug("Suppressed duplicate/no-op local CDM observation",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.String("source_position", sourcePosition),
			slog.String("source_role", sourceRole),
		)
		return nil
	}

	if len(appliedOverrides) > 0 {
		updated.Pending = nil
	}

	rows, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated.Normalize())
	if err != nil {
		return err
	}

	if rows != 1 {
		return fmt.Errorf("failed to persist local CDM observation for %s session %d", callsign, session)
	}

	slog.Info("Merged local CDM observation",
		slog.Int("session", int(session)),
		slog.String("callsign", callsign),
		slog.String("source_position", sourcePosition),
		slog.String("source_role", sourceRole),
		slog.Any("applied_overrides", appliedOverrides),
		slog.Any("plugin_updates", pluginUpdates),
		slog.String("tobt", helpers.ValueOrDefault(updated.EffectiveTobt())),
		slog.String("tsat", helpers.ValueOrDefault(updated.EffectiveTsat())),
		slog.String("ttot", helpers.ValueOrDefault(updated.EffectiveTtot())),
		slog.String("ctot", helpers.ValueOrDefault(updated.EffectiveCtot())),
	)

	s.broadcastEffectiveCdmIfChanged(session, callsign, before, snapshotEffectiveCdm(updated))

	return nil
}

func (s *Service) HandleReadyRequest(ctx context.Context, session int32, callsign string) error {
	if !s.client.isValid {
		return nil
	}

	sessionData, err := s.sessionRepo.GetByID(ctx, session)
	if err != nil {
		return err
	}

	airportMaster, err := s.client.AirportMasterByICAO(ctx, sessionData.Airport)
	if err != nil {
		return err
	}

	if airportMaster == nil {
		slog.Info("CDM ready request skipped: no external CDM master configured for airport",
			slog.Int("session", int(session)),
			slog.String("airport", sessionData.Airport),
			slog.String("callsign", callsign),
			slog.String("branch", "no-master"),
		)
		return nil
	}

	slog.Info("Resolved external CDM master for airport",
		slog.Int("session", int(session)),
		slog.String("airport", sessionData.Airport),
		slog.String("callsign", callsign),
		slog.String("master_position", airportMaster.Position),
	)

	controller, err := s.controllerRepo.GetByCallsign(ctx, session, airportMaster.Position)
	switch {
	case err == nil && controller.Cid != nil && *controller.Cid != "" && s.euroscopeHub != nil && os.Getenv("CDM_ES_FAST_PATH") == "true":
		slog.Info("Handling CDM ready request via targeted EuroScope fast path",
			slog.Int("session", int(session)),
			slog.String("airport", sessionData.Airport),
			slog.String("callsign", callsign),
			slog.String("branch", "fast-es"),
			slog.String("target_position", airportMaster.Position),
			slog.String("target_cid", *controller.Cid),
		)

		if err := s.persistPendingReadyRequest(ctx, session, callsign, "euroscope", &airportMaster.Position); err != nil {
			return err
		}

		if s.frontendHub != nil {
			s.frontendHub.SendCdmWait(session, callsign)
		}

		s.euroscopeHub.SendCdmReadyRequest(session, *controller.Cid, callsign)
		slog.Info("Sent targeted CDM ready request to EuroScope client",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.String("target_position", airportMaster.Position),
			slog.String("target_cid", *controller.Cid),
		)
		return nil
	case err != nil && !errors.Is(err, pgx.ErrNoRows):
		return err
	default:
		slog.Info("Falling back to IFPS CDM ready request: external master not connected locally",
			slog.Int("session", int(session)),
			slog.String("airport", sessionData.Airport),
			slog.String("callsign", callsign),
			slog.String("branch", "fallback-api"),
			slog.String("target_position", airportMaster.Position),
		)
		return s.RequestBetterTobt(ctx, session, callsign)
	}
}

func (s *Service) SetReady(ctx context.Context, session int32, callsign string) error {
	if !s.client.isValid {
		return nil
	}
	cdmData, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if cdmData.EffectiveStatus() != nil && *cdmData.EffectiveStatus() == "REA" {
		return nil
	}

	if err := s.client.IFPSSetCDMStatus(ctx, callsign, "REA"); err != nil {
		return err
	}

	rea := "REA"
	updated := cdmData.Clone()
	updated.Canonical.Status = &rea
	rows, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated)
	if err != nil {
		return err
	}

	if rows != 1 {
		return fmt.Errorf("failed to update CDM status for %s session %d", callsign, session)
	}

	s.frontendHub.SendCdmWait(session, callsign)

	return nil
}

func (s *Service) RequestBetterTobt(ctx context.Context, session int32, callsign string) error {
	if !s.client.isValid {
		return nil
	}
	now := time.Now()
	// format hhmm
	format := now.Format("1504")
	status := "REQTOBT/" + format + "/ATC"

	cdmData, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	if cdmData.EffectiveStatus() != nil && *cdmData.EffectiveStatus() == status {
		return nil
	}

	err = s.client.IFPSSetCDMStatus(ctx, callsign, status)
	if err != nil {
		return err
	}

	updated := cdmData.Clone()
	updated.Canonical.Status = &status
	rows, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated)
	if err != nil {
		return err
	}

	if rows != 1 {
		return fmt.Errorf("failed to update CDM status for %s session %d", callsign, session)
	}

	s.frontendHub.SendCdmWait(session, callsign)

	return nil
}

func (s *Service) persistPendingReadyRequest(ctx context.Context, session int32, callsign string, via string, targetPosition *string) error {
	cdmData, err := s.stripRepo.GetCdmDataForCallsign(ctx, session, callsign)
	if err != nil {
		return err
	}

	updated := cdmData.Clone()
	now := time.Now().UTC()
	updated.Pending = &models.CdmPendingRequest{
		RequestedAt:    &now,
		Via:            via,
		TargetPosition: targetPosition,
	}

	rows, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated)
	if err != nil {
		return err
	}

	if rows != 1 {
		return fmt.Errorf("failed to persist pending CDM request for %s session %d", callsign, session)
	}

	return nil
}

func (s *Service) applyCanonicalCdmState(ctx context.Context, session int32, callsign string, flight *models.CdmData, next canonicalCdmState) error {
	before := snapshotEffectiveCdm(flight)
	updated := flight.Clone()
	updated.Canonical.Tobt = &next.Tobt
	updated.Canonical.Tsat = &next.Tsat
	updated.Canonical.Ttot = &next.Ttot
	updated.Canonical.Ctot = &next.Ctot
	updated.Canonical.Aobt = &next.Aobt
	updated.Canonical.Eobt = &next.Eobt
	updated.Canonical.Status = &next.Status
	updated.Canonical.Source = next.Source
	now := time.Now().UTC()
	updated.Canonical.UpdatedAt = &now
	updated.Pending = nil
	clearedOverrides := make([]string, 0, 4)
	clearMatchingOverride(updated, "tobt", updated.Canonical.Tobt, &clearedOverrides)
	clearMatchingOverride(updated, "tsat", updated.Canonical.Tsat, &clearedOverrides)
	clearMatchingOverride(updated, "ttot", updated.Canonical.Ttot, &clearedOverrides)
	clearMatchingOverride(updated, "ctot", updated.Canonical.Ctot, &clearedOverrides)
	logExpiredOverrides(session, callsign, updated, now)

	if _, err := s.stripRepo.SetCdmData(ctx, session, callsign, updated.Normalize()); err != nil {
		return err
	}

	if len(clearedOverrides) > 0 {
		slog.Info("Cleared reconciled local CDM overrides",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.Any("cleared_overrides", clearedOverrides),
			slog.String("source", next.Source),
		)
	}

	s.broadcastEffectiveCdmIfChanged(session, callsign, before, snapshotEffectiveCdm(updated))

	return nil
}

func (s *Service) Start(ctx context.Context) {
	if !s.client.isValid {
		slog.Warn("CDM client is not valid, CDM data will not be synced")
		return
	}

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		sessions, err := s.sessionRepo.List(ctx)
		if err != nil {
			continue
		}
		for _, session := range sessions {
			if session.Name != "LIVE" {
				continue
			}

			slog.Debug("Syncing CDM data", slog.String("session", session.Name), slog.Int("id", int(session.ID)), slog.String("airport", session.Airport))

			err = s.syncCdmData(ctx, session)
			if err != nil {
				slog.Error("Failed to sync CDM data", slog.Any("error", err))
			}
		}
	}
}

func (s *Service) syncCdmData(ctx context.Context, session *models.Session) error {
	if !s.client.isValid {
		return nil
	}

	airport := session.Airport

	currentData, err := s.stripRepo.GetCdmData(ctx, session.ID)
	if err != nil {
		return err
	}

	lookup := make(map[string]*models.CdmData)
	for _, row := range currentData {
		lookup[row.Callsign] = row.Data
	}

	newData, err := s.client.IFPSByDepartureAirport(ctx, airport)
	if err != nil {
		return err
	}

	for _, row := range newData {
		if flight, ok := lookup[row.Callsign]; ok {
			next := canonicalCdmState{
				Tobt:   row.TOBT,
				Tsat:   truncateCDMClockValue(row.CDMData.TSAT),
				Ttot:   truncateCDMClockValue(row.CDMData.TTOT),
				Ctot:   row.CTOT,
				Aobt:   row.AOBT,
				Eobt:   row.EOBT,
				Status: row.CDMStatus,
				Source: "ifps",
			}
			if helpers.ValueOrDefault(flight.Canonical.Status) != next.Status ||
				helpers.ValueOrDefault(flight.Canonical.Aobt) != next.Aobt ||
				helpers.ValueOrDefault(flight.Canonical.Eobt) != next.Eobt ||
				helpers.ValueOrDefault(flight.Canonical.Ctot) != next.Ctot ||
				helpers.ValueOrDefault(flight.Canonical.Tobt) != next.Tobt ||
				helpers.ValueOrDefault(flight.Canonical.Tsat) != next.Tsat ||
				helpers.ValueOrDefault(flight.Canonical.Ttot) != next.Ttot {
				if err := s.applyCanonicalCdmState(ctx, session.ID, row.Callsign, flight, next); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func truncateCDMClockValue(value string) string {
	if len(value) > 4 {
		return value[:4]
	}

	return value
}

func snapshotEffectiveCdm(data *models.CdmData) effectiveCdmSnapshot {
	return effectiveCdmSnapshot{
		Eobt: helpers.ValueOrDefault(data.EffectiveEobt()),
		Tobt: helpers.ValueOrDefault(data.EffectiveTobt()),
		Tsat: helpers.ValueOrDefault(data.EffectiveTsat()),
		Ctot: helpers.ValueOrDefault(data.EffectiveCtot()),
	}
}

func (s *Service) broadcastEffectiveCdmIfChanged(session int32, callsign string, before, after effectiveCdmSnapshot) {
	if s.frontendHub == nil {
		return
	}

	if before == after {
		return
	}

	s.frontendHub.SendCdmUpdate(session, callsign, after.Eobt, after.Tobt, after.Tsat, after.Ctot)
}

func applyLocalOverride(
	data *models.CdmData,
	field string,
	value string,
	sourcePosition string,
	sourceRole string,
	observedAt time.Time,
	expiresAt time.Time,
	appliedFields *[]string,
) {
	if value == "" {
		return
	}

	if existing, ok := data.LocalOverrides[field]; ok &&
		existing.Value == value &&
		existing.SourcePosition == sourcePosition &&
		existing.SourceRole == sourceRole {
		return
	}

	if data.LocalOverrides == nil {
		data.LocalOverrides = make(map[string]models.CdmFieldOverride)
	}

	expiresAtCopy := expiresAt
	data.LocalOverrides[field] = models.CdmFieldOverride{
		Value:          value,
		ObservedAt:     observedAt,
		SourcePosition: sourcePosition,
		SourceRole:     sourceRole,
		ExpiresAt:      &expiresAtCopy,
	}
	*appliedFields = append(*appliedFields, field)
}

func setPluginField(target **string, value string, fieldName string, changedFields *[]string) {
	if value == "" {
		return
	}

	if *target != nil && **target == value {
		return
	}

	next := value
	*target = &next
	*changedFields = append(*changedFields, fieldName)
}

func clearMatchingOverride(data *models.CdmData, field string, canonical *string, clearedFields *[]string) {
	if data == nil || len(data.LocalOverrides) == 0 || canonical == nil {
		return
	}

	override, ok := data.LocalOverrides[field]
	if !ok || override.Value != *canonical {
		return
	}

	data.ClearMatchingLocalOverride(field, canonical)
	*clearedFields = append(*clearedFields, field)
}

func logExpiredOverrides(session int32, callsign string, data *models.CdmData, now time.Time) {
	if data == nil {
		return
	}

	for field, override := range data.LocalOverrides {
		if override.ExpiresAt == nil || now.Before(*override.ExpiresAt) {
			continue
		}

		slog.Warn("Local CDM override is past TTL and awaiting reconciliation",
			slog.Int("session", int(session)),
			slog.String("callsign", callsign),
			slog.String("field", field),
			slog.String("value", override.Value),
			slog.String("source_position", override.SourcePosition),
			slog.String("source_role", override.SourceRole),
			slog.Time("observed_at", override.ObservedAt),
			slog.Time("expires_at", *override.ExpiresAt),
		)
	}
}
