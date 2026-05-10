package cdm

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

type WebAPI struct {
	authenticationService shared.AuthenticationService
	sessionRepo           repository.SessionRepository
	sequenceService       *SequenceService
	now                   func() time.Time
}

type sequenceResponse struct {
	GeneratedAt string                    `json:"generated_at"`
	Sessions    []sequenceSessionResponse `json:"sessions"`
}

type sequenceSessionResponse struct {
	SessionID        int32                 `json:"session_id"`
	Name             string                `json:"name"`
	Airport          string                `json:"airport"`
	CdmMaster        bool                  `json:"cdm_master"`
	DepartureRunways []string              `json:"departure_runways"`
	ArrivalRunways   []string              `json:"arrival_runways"`
	Rows             []sequenceRowResponse `json:"rows"`
}

type sequenceRowResponse struct {
	Position        *int                     `json:"position,omitempty"`
	Callsign        string                   `json:"callsign"`
	Origin          string                   `json:"origin"`
	Destination     string                   `json:"destination"`
	Runway          string                   `json:"runway"`
	Sid             string                   `json:"sid"`
	WakeCategory    string                   `json:"wake_category"`
	State           string                   `json:"state"`
	Eobt            string                   `json:"eobt"`
	Tobt            string                   `json:"tobt"`
	ReqTobt         string                   `json:"req_tobt"`
	TobtConfirmed   bool                     `json:"tobt_confirmed"`
	TobtConfirmedBy string                   `json:"tobt_confirmed_by"`
	Tsat            string                   `json:"tsat"`
	Ttot            string                   `json:"ttot"`
	NaturalTtot     string                   `json:"natural_ttot"`
	TaxiMinutes     *int                     `json:"taxi_minutes,omitempty"`
	TaxiRunway      string                   `json:"taxi_runway"`
	Ctot            string                   `json:"ctot"`
	BaseTime        string                   `json:"base_time"`
	BaseSource      string                   `json:"base_source"`
	Phase           string                   `json:"phase"`
	InvalidReason   string                   `json:"invalid_reason"`
	Reasons         []sequenceReasonResponse `json:"reasons"`
}

type sequenceReasonResponse struct {
	Kind            string `json:"kind"`
	Message         string `json:"message"`
	AgainstCallsign string `json:"against_callsign,omitempty"`
}

type sequenceSnapshotRow struct {
	response sequenceRowResponse
	sortTTOT string
	sortBase string
}

func NewWebAPI(authenticationService shared.AuthenticationService, sessionRepo repository.SessionRepository, sequenceService *SequenceService) *WebAPI {
	return &WebAPI{
		authenticationService: authenticationService,
		sessionRepo:           sessionRepo,
		sequenceService:       sequenceService,
		now:                   time.Now,
	}
}

func (a *WebAPI) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/cdm/sequence", a.handleSequence)
}

func (a *WebAPI) handleSequence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	if _, ok := a.authenticate(w, r); !ok {
		return
	}

	if a.sessionRepo == nil || a.sequenceService == nil {
		writeJSONError(w, http.StatusServiceUnavailable, "cdm sequence unavailable")
		return
	}

	sessions, err := a.sessionRepo.List(r.Context())
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to list sessions")
		return
	}

	now := a.now().UTC()
	response := sequenceResponse{
		GeneratedAt: now.Format(time.RFC3339),
		Sessions:    make([]sequenceSessionResponse, 0, len(sessions)),
	}

	for _, session := range sessions {
		if session == nil {
			continue
		}

		snapshot, err := buildSequenceSessionSnapshot(r.Context(), a.sequenceService, session, now)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "failed to build cdm sequence")
			return
		}
		if len(snapshot.Rows) == 0 {
			continue
		}

		response.Sessions = append(response.Sessions, snapshot)
	}

	sort.SliceStable(response.Sessions, func(i, j int) bool {
		left := response.Sessions[i]
		right := response.Sessions[j]
		if left.Airport != right.Airport {
			return left.Airport < right.Airport
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		return left.SessionID < right.SessionID
	})

	writeJSON(w, http.StatusOK, response)
}

func buildSequenceSessionSnapshot(ctx context.Context, service *SequenceService, session *models.Session, now time.Time) (sequenceSessionResponse, error) {
	response := sequenceSessionResponse{
		SessionID:        session.ID,
		Name:             session.Name,
		Airport:          session.Airport,
		CdmMaster:        session.CdmMaster,
		DepartureRunways: append([]string(nil), session.ActiveRunways.DepartureRunways...),
		ArrivalRunways:   append([]string(nil), session.ActiveRunways.ArrivalRunways...),
		Rows:             []sequenceRowResponse{},
	}

	strips, err := service.stripRepo.ListByOrigin(ctx, session.ID, session.Airport)
	if err != nil {
		return response, err
	}

	rows := buildPersistedSequenceRows(strips, session.CdmMaster, now)
	response.Rows = make([]sequenceRowResponse, len(rows))
	for i, row := range rows {
		response.Rows[i] = row.response
	}

	return response, nil
}

func buildSequenceSnapshotRows(strips []*models.Strip, config *CdmAirportConfig, now time.Time) []sequenceSnapshotRow {
	candidates := make([]sequencingCandidate, 0, len(strips))
	for _, strip := range strips {
		if strip == nil || shouldSkipStrip(strip) {
			continue
		}

		calcInput := buildCalcInput(strip, config)
		slot, hasSlot := existingSlotEntry(strip)
		candidates = append(candidates, sequencingCandidate{
			strip:       strip,
			input:       calcInput,
			baseTime:    selectCalculationBase(calcInput),
			naturalTtot: unconstrainedTtot(calcInput, config, now),
			slot:        slot,
			hasSlot:     hasSlot,
			hasCtot:     strings.TrimSpace(valueOrEmpty(strip.EffectiveCtot())) != "",
			started:     valueOrEmpty(strip.EffectiveAsat()) != "" || valueOrEmpty(strip.EffectiveAobt()) != "",
			staleBase:   shouldInvalidateStaleTobt(calcInput, timeToClock(now)),
		})
	}

	preserved := make([]sequencingCandidate, 0, len(candidates))
	recalculate := make([]sequencingCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if shouldRecalculateStrip(candidate.strip, now) || !candidate.hasSlot {
			recalculate = append(recalculate, candidate)
			continue
		}
		preserved = append(preserved, candidate)
	}

	sort.SliceStable(preserved, func(i, j int) bool {
		return compareSequencingCandidates(preserved[i], preserved[j], now, true) < 0
	})

	slots := make([]SlotEntry, 0, len(candidates))
	rows := make([]sequenceSnapshotRow, 0, len(candidates))
	for _, candidate := range preserved {
		if candidate.started {
			slots = append(slots, candidate.slot)
			rows = append(rows, buildPreservedSnapshotRow(candidate, candidate.slot))
			continue
		}
		if !hasPreservedSlotConflict(candidate.slot, slots, config) &&
			!preservedSlotBlocksHigherPriorityCandidate(candidate, recalculate, config, now) {
			slots = append(slots, candidate.slot)
			rows = append(rows, buildPreservedSnapshotRow(candidate, candidate.slot))
			continue
		}
		recalculate = append(recalculate, candidate)
	}

	sort.SliceStable(recalculate, func(i, j int) bool {
		return compareSequencingCandidates(recalculate[i], recalculate[j], now, false) < 0
	})

	for _, candidate := range recalculate {
		if !shouldRecalculateStrip(candidate.strip, now) {
			if slot, ok := existingSlotEntry(candidate.strip); ok {
				if candidate.started || canRetainExistingSlot(candidate, slot, slots, config, now) {
					slots = append(slots, slot)
					rows = append(rows, buildPreservedSnapshotRow(candidate, slot))
				}
			}
			continue
		}

		if isTsatSpecificallyExpired(candidate.strip, now) {
			updated := cloneSequenceData(candidate.strip.CdmData)
			phase := "I"
			updated.Phase = &phase
			updated.Tsat = nil
			updated.Ttot = nil
			applyCalculationSnapshot(updated, candidate.input, valueOrEmpty(candidate.strip.Runway), models.CdmInvalidReasonStaleTsat)
			updated.ClearLocalRecalculationPending()
			rows = append(rows, buildCalculatedSnapshotRow(candidate, updated, CalcResult{}, nil, false))
			continue
		}

		result, trace := calculateWithTrace(candidate.input, slots, config, now)
		updated := cloneSequenceData(candidate.strip.CdmData)
		updated.Phase = nil
		updated.Tsat = stringPointerIfPresent(result.Tsat)
		updated.Ttot = stringPointerIfPresent(result.Ttot)
		applyCalculationSnapshot(updated, candidate.input, valueOrEmpty(candidate.strip.Runway), calculationInvalidReason(candidate.input, result, now))
		updated.ClearLocalRecalculationPending()

		rows = append(rows, buildCalculatedSnapshotRow(candidate, updated, result, trace, true))
		if result.Ttot != "" {
			slots = append(slots, SlotEntry{
				Callsign:    candidate.strip.Callsign,
				Origin:      candidate.strip.Origin,
				Destination: candidate.strip.Destination,
				DepRwy:      valueOrEmpty(candidate.strip.Runway),
				Sid:         valueOrEmpty(candidate.strip.Sid),
				WakeCat:     valueOrEmpty(candidate.strip.AircraftCategory),
				Ttot:        result.Ttot,
				HasManCtot:  updated.HasManualCtot(),
				ManCtot:     valueOrEmpty(updated.Ctot),
			})
		}
	}

	assignSequencePositions(rows, now)
	return rows
}

func buildPersistedSequenceRows(strips []*models.Strip, isMaster bool, now time.Time) []sequenceSnapshotRow {
	rows := make([]sequenceSnapshotRow, 0, len(strips))
	for _, strip := range strips {
		if strip == nil || shouldSkipStrip(strip) {
			continue
		}

		data := cloneSequenceData(strip.CdmData)
		baseTime, baseSource := remoteSequenceBase(data)
		calculation := data.EffectiveCalculation()
		response := sequenceRowResponse{
			Position:        storedSequencePosition(calculation),
			Callsign:        strip.Callsign,
			Origin:          strip.Origin,
			Destination:     strip.Destination,
			Runway:          valueOrEmpty(strip.Runway),
			Sid:             valueOrEmpty(strip.Sid),
			WakeCategory:    valueOrEmpty(strip.AircraftCategory),
			State:           valueOrEmpty(strip.State),
			Eobt:            truncateSequenceClock(valueOrEmpty(data.EffectiveEobt())),
			Tobt:            truncateSequenceClock(valueOrEmpty(data.EffectiveTobt())),
			ReqTobt:         truncateSequenceClock(valueOrEmpty(data.EffectiveReqTobt())),
			TobtConfirmed:   valueOrEmpty(data.TobtConfirmedBy) != "",
			TobtConfirmedBy: valueOrEmpty(data.TobtConfirmedBy),
			Tsat:            truncateSequenceClock(valueOrEmpty(data.EffectiveTsat())),
			Ttot:            truncateSequenceClock(valueOrEmpty(data.EffectiveTtot())),
			NaturalTtot:     "",
			TaxiMinutes:     storedTaxiMinutes(calculation),
			TaxiRunway:      storedTaxiRunway(calculation, valueOrEmpty(strip.Runway)),
			Ctot:            truncateSequenceClock(valueOrEmpty(data.EffectiveCtot())),
			BaseTime:        truncateSequenceClock(baseTime),
			BaseSource:      baseSource,
			Phase:           valueOrEmpty(data.EffectivePhase()),
			InvalidReason:   remoteSequenceInvalidReason(data),
			Reasons:         storedReasonResponses(calculation, data, isMaster),
		}

		rows = append(rows, sequenceSnapshotRow{
			response: response,
			sortTTOT: valueOrEmpty(data.EffectiveTtot()),
			sortBase: baseTime,
		})
	}

	sortPersistedRows(rows, now)
	return rows
}

func buildPreservedSnapshotRow(candidate sequencingCandidate, slot SlotEntry) sequenceSnapshotRow {
	data := cloneSequenceData(candidate.strip.CdmData)
	reasons := buildPreservedReasons(candidate, slot, data)

	return sequenceSnapshotRow{
		response: buildSnapshotResponse(candidate, data, truncateSequenceClock(slot.Ttot), truncateSequenceClock(slot.Ttot), reasons),
		sortTTOT: slot.Ttot,
		sortBase: candidate.baseTime,
	}
}

func buildCalculatedSnapshotRow(candidate sequencingCandidate, data *models.CdmData, result CalcResult, trace []calculationTraceEntry, includeNaturalReason bool) sequenceSnapshotRow {
	ttot := truncateSequenceClock(result.Ttot)
	reasons := buildCalculatedReasons(candidate, data, result, trace, includeNaturalReason)

	return sequenceSnapshotRow{
		response: buildSnapshotResponse(candidate, data, ttot, truncateSequenceClock(candidate.naturalTtot), reasons),
		sortTTOT: result.Ttot,
		sortBase: candidate.baseTime,
	}
}

func buildSnapshotResponse(candidate sequencingCandidate, data *models.CdmData, ttot string, naturalTtot string, reasons []sequenceReasonResponse) sequenceRowResponse {
	baseTime, baseSource := selectCalculationBaseWithSource(candidate.input)
	invalidReason := ""
	if calculation := data.EffectiveCalculation(); calculation != nil {
		invalidReason = valueOrEmpty(calculation.InvalidReason)
	}

	confirmedBy := valueOrEmpty(data.TobtConfirmedBy)
	return sequenceRowResponse{
		Callsign:        candidate.strip.Callsign,
		Origin:          candidate.strip.Origin,
		Destination:     candidate.strip.Destination,
		Runway:          valueOrEmpty(candidate.strip.Runway),
		Sid:             valueOrEmpty(candidate.strip.Sid),
		WakeCategory:    valueOrEmpty(candidate.strip.AircraftCategory),
		State:           valueOrEmpty(candidate.strip.State),
		Eobt:            truncateSequenceClock(valueOrEmpty(data.EffectiveEobt())),
		Tobt:            truncateSequenceClock(valueOrEmpty(data.EffectiveTobt())),
		ReqTobt:         truncateSequenceClock(valueOrEmpty(data.EffectiveReqTobt())),
		TobtConfirmed:   confirmedBy != "",
		TobtConfirmedBy: confirmedBy,
		Tsat:            truncateSequenceClock(valueOrEmpty(data.EffectiveTsat())),
		Ttot:            ttot,
		NaturalTtot:     naturalTtot,
		Ctot:            truncateSequenceClock(valueOrEmpty(data.EffectiveCtot())),
		BaseTime:        truncateSequenceClock(baseTime),
		BaseSource:      baseSource,
		Phase:           valueOrEmpty(data.EffectivePhase()),
		InvalidReason:   invalidReason,
		Reasons:         reasons,
	}
}

func buildPreservedReasons(candidate sequencingCandidate, slot SlotEntry, data *models.CdmData) []sequenceReasonResponse {
	reasons := make([]sequenceReasonResponse, 0, 4)
	reasons = append(reasons, buildBaseReason(candidate))
	if candidate.hasCtot {
		reasons = append(reasons, sequenceReasonResponse{
			Kind:    "ctot_priority",
			Message: fmt.Sprintf("CTOT %s keeps this flight ahead of non-CTOT traffic where constraints conflict.", truncateSequenceClock(valueOrEmpty(data.EffectiveCtot()))),
		})
	}
	if candidate.started {
		reasons = append(reasons, sequenceReasonResponse{
			Kind:    "started_flight",
			Message: fmt.Sprintf("The flight has already started, so its existing TTOT %s is preserved.", truncateSequenceClock(slot.Ttot)),
		})
	} else {
		reasons = append(reasons, sequenceReasonResponse{
			Kind:    "preserved_slot",
			Message: fmt.Sprintf("The existing TTOT %s still fits the current sequence, so the slot is preserved.", truncateSequenceClock(slot.Ttot)),
		})
	}
	if candidate.staleBase && selectCalculationBase(candidate.input) != normalizeCalculationClock(candidate.input.Tobt) {
		reasons = append(reasons, sequenceReasonResponse{
			Kind:    "stale_tobt_fallback",
			Message: "A stale TOBT is not used as the sequencing base, so the sequence falls back to a later valid requested/EOBT time.",
		})
	}
	return reasons
}

func buildCalculatedReasons(candidate sequencingCandidate, data *models.CdmData, result CalcResult, trace []calculationTraceEntry, includeNaturalReason bool) []sequenceReasonResponse {
	reasons := make([]sequenceReasonResponse, 0, len(trace)+4)
	reasons = append(reasons, buildBaseReason(candidate))

	if candidate.hasCtot {
		reasons = append(reasons, sequenceReasonResponse{
			Kind:    "ctot_priority",
			Message: fmt.Sprintf("CTOT %s gives this flight sequencing priority before non-CTOT traffic.", truncateSequenceClock(valueOrEmpty(data.EffectiveCtot()))),
		})
	}

	if candidate.staleBase && selectCalculationBase(candidate.input) != normalizeCalculationClock(candidate.input.Tobt) {
		reasons = append(reasons, sequenceReasonResponse{
			Kind:    "stale_tobt_fallback",
			Message: "A stale TOBT is not used as the sequencing base, so the sequence falls back to a later valid requested/EOBT time.",
		})
	}

	if result.Ttot == "" {
		reasons = append(reasons, buildInvalidReason(data))
		return reasons
	}

	if includeNaturalReason && len(trace) == 0 {
		reasons = append(reasons, sequenceReasonResponse{
			Kind:    "natural_slot",
			Message: fmt.Sprintf("No spacing constraint changed the natural TTOT %s, so the flight keeps its calculated slot.", truncateSequenceClock(result.Ttot)),
		})
	}

	for _, entry := range trace {
		reasons = append(reasons, sequenceReasonResponse{
			Kind:            entry.Kind,
			AgainstCallsign: entry.AgainstCallsign,
			Message:         buildTraceReasonMessage(entry),
		})
	}

	return reasons
}

func buildBaseReason(candidate sequencingCandidate) sequenceReasonResponse {
	baseTime, baseSource := selectCalculationBaseWithSource(candidate.input)
	sourceLabel := baseSource
	switch baseSource {
	case models.CdmCalculationBaseTobt:
		sourceLabel = "TOBT"
	case models.CdmCalculationBaseReqTobt:
		sourceLabel = "REQ TOBT"
	case models.CdmCalculationBaseEobt:
		sourceLabel = "EOBT"
	}

	taxiParts := []string{}
	if candidate.input.TaxiMin > 0 {
		taxiParts = append(taxiParts, strconv.Itoa(candidate.input.TaxiMin)+" min taxi")
	}
	if candidate.input.DeIceMin > 0 {
		taxiParts = append(taxiParts, strconv.Itoa(candidate.input.DeIceMin)+" min de-ice")
	}

	message := fmt.Sprintf("Base time %s %s drives the natural TTOT %s.", sourceLabel, truncateSequenceClock(baseTime), truncateSequenceClock(candidate.naturalTtot))
	if len(taxiParts) > 0 {
		message = fmt.Sprintf("Base time %s %s plus %s drives the natural TTOT %s.", sourceLabel, truncateSequenceClock(baseTime), strings.Join(taxiParts, " and "), truncateSequenceClock(candidate.naturalTtot))
	}

	return sequenceReasonResponse{
		Kind:    "base_time",
		Message: message,
	}
}

func buildInvalidReason(data *models.CdmData) sequenceReasonResponse {
	invalidReason := ""
	if calculation := data.EffectiveCalculation(); calculation != nil {
		invalidReason = valueOrEmpty(calculation.InvalidReason)
	}

	switch invalidReason {
	case models.CdmInvalidReasonStaleTsat:
		return sequenceReasonResponse{
			Kind:    "stale_tsat",
			Message: "The previous TSAT has expired, so the flight is waiting for a fresh recalculation.",
		}
	case models.CdmInvalidReasonStaleTobt:
		return sequenceReasonResponse{
			Kind:    "stale_tobt",
			Message: "The TOBT is more than 5 minutes in the past, so the flight is waiting for an updated TOBT before it can be resequenced.",
		}
	default:
		return sequenceReasonResponse{
			Kind:    "unsequenced",
			Message: "The flight does not currently have a valid TTOT for sequencing.",
		}
	}
}

func buildTraceReasonMessage(entry calculationTraceEntry) string {
	against := entry.AgainstCallsign
	if against == "" {
		against = "another flight"
	}

	switch entry.Kind {
	case "same_destination_separation":
		return fmt.Sprintf("Moved behind %s to keep %.0f minutes of same-destination spacing, shifting TTOT from %s to %s.", against, entry.RequiredSpacingMinutes, truncateSequenceClock(entry.FromTtot), truncateSequenceClock(entry.ToTtot))
	case "runway_slot_collision":
		return fmt.Sprintf("Moved behind %s because both flights would otherwise use the same runway slot at %s.", against, truncateSequenceClock(entry.AgainstTtot))
	case "runway_rate_window":
		return fmt.Sprintf("Moved behind %s to stay outside the runway rate window (%.1f minutes), shifting TTOT from %s to %s.", against, entry.RequiredSpacingMinutes, truncateSequenceClock(entry.FromTtot), truncateSequenceClock(entry.ToTtot))
	case "wake_separation":
		return fmt.Sprintf("Moved behind %s to satisfy wake separation on runway %s, shifting TTOT from %s to %s.", against, strings.TrimSpace(entry.AgainstRunway), truncateSequenceClock(entry.FromTtot), truncateSequenceClock(entry.ToTtot))
	case "sid_interval":
		return fmt.Sprintf("Moved behind %s to satisfy the SID interval on runway %s, shifting TTOT from %s to %s.", against, strings.TrimSpace(entry.AgainstRunway), truncateSequenceClock(entry.FromTtot), truncateSequenceClock(entry.ToTtot))
	default:
		return fmt.Sprintf("Moved behind %s due to a sequencing constraint, shifting TTOT from %s to %s.", against, truncateSequenceClock(entry.FromTtot), truncateSequenceClock(entry.ToTtot))
	}
}

func remoteSequenceBase(data *models.CdmData) (string, string) {
	if data == nil {
		return "", ""
	}
	if tobt := normalizeCalculationClock(valueOrEmpty(data.EffectiveTobt())); tobt != "" {
		return tobt, models.CdmCalculationBaseTobt
	}
	if reqTobt := normalizeCalculationClock(valueOrEmpty(data.EffectiveReqTobt())); reqTobt != "" {
		return reqTobt, models.CdmCalculationBaseReqTobt
	}
	if eobt := normalizeCalculationClock(valueOrEmpty(data.EffectiveEobt())); eobt != "" {
		return eobt, models.CdmCalculationBaseEobt
	}
	return "", ""
}

func remoteSequenceInvalidReason(data *models.CdmData) string {
	if data == nil {
		return ""
	}
	if calculation := data.EffectiveCalculation(); calculation != nil {
		return valueOrEmpty(calculation.InvalidReason)
	}
	return ""
}

func storedReasonResponses(calculation *models.CdmCalculation, data *models.CdmData, isMaster bool) []sequenceReasonResponse {
	if calculation != nil && len(calculation.ReasonMarkers) > 0 {
		reasons := make([]sequenceReasonResponse, 0, len(calculation.ReasonMarkers))
		for _, marker := range calculation.ReasonMarkers {
			if !isInterestingReasonKind(marker.Kind) {
				continue
			}
			message := buildStoredReasonMarkerMessage(marker)
			if message == "" {
				continue
			}
			reasons = append(reasons, sequenceReasonResponse{
				Kind:            marker.Kind,
				Message:         message,
				AgainstCallsign: valueOrEmpty(marker.AgainstCallsign),
			})
		}
		if len(reasons) > 0 {
			return reasons
		}
	}

	reasons := make([]sequenceReasonResponse, 0, 1)
	if data != nil && remoteSequenceInvalidReason(data) != "" {
		reasons = append(reasons, buildInvalidReason(data))
	}
	return reasons
}

func isInterestingReasonKind(kind string) bool {
	switch kind {
	case "same_destination_separation", "runway_slot_collision", "runway_rate_window", "runway_spacing", "wake_separation", "sid_interval", "stale_tobt", "stale_tsat":
		return true
	default:
		return false
	}
}

func buildStoredReasonMarkerMessage(marker models.CdmReasonMarker) string {
	if strings.TrimSpace(marker.Message) != "" {
		return marker.Message
	}

	against := valueOrEmpty(marker.AgainstCallsign)
	if against == "" {
		against = "another flight"
	}
	from := truncateSequenceClock(valueOrEmpty(marker.FromTtot))
	to := truncateSequenceClock(valueOrEmpty(marker.ToTtot))
	runway := strings.TrimSpace(valueOrEmpty(marker.AgainstRunway))
	spacing := 0.0
	if marker.RequiredSpacingMinutes != nil {
		spacing = *marker.RequiredSpacingMinutes
	}

	switch marker.Kind {
	case "same_destination_separation":
		return buildStoredShiftMessage(fmt.Sprintf("Moved behind %s to keep %.0f minutes of same-destination spacing", against, spacing), from, to)
	case "runway_slot_collision":
		return buildStoredShiftMessage(fmt.Sprintf("Moved behind %s because both flights would otherwise use the same runway slot at %s", against, truncateSequenceClock(valueOrEmpty(marker.AgainstTtot))), from, to)
	case "runway_rate_window":
		return buildStoredShiftMessage(fmt.Sprintf("Moved behind %s to stay outside the runway rate window (%.1f minutes)", against, spacing), from, to)
	case "runway_spacing":
		return buildStoredShiftMessage(fmt.Sprintf("Moved behind %s due to runway departure spacing", against), from, to)
	case "wake_separation":
		if runway != "" {
			return buildStoredShiftMessage(fmt.Sprintf("Moved behind %s to satisfy wake separation on runway %s", against, runway), from, to)
		}
		return buildStoredShiftMessage(fmt.Sprintf("Moved behind %s to satisfy wake separation", against), from, to)
	case "sid_interval":
		if runway != "" {
			return buildStoredShiftMessage(fmt.Sprintf("Moved behind %s to satisfy the SID interval on runway %s", against, runway), from, to)
		}
		return buildStoredShiftMessage(fmt.Sprintf("Moved behind %s to satisfy the SID interval", against), from, to)
	case "stale_tobt":
		return "The TOBT is more than 5 minutes in the past, so the flight is waiting for an updated TOBT before it can be resequenced."
	case "stale_tsat":
		return "The previous TSAT has expired, so the flight is waiting for a fresh recalculation."
	default:
		return ""
	}
}

func buildStoredShiftMessage(prefix string, from string, to string) string {
	if from != "" && to != "" && from != to {
		return fmt.Sprintf("%s, shifting TTOT from %s to %s.", prefix, from, to)
	}
	if to != "" {
		return fmt.Sprintf("%s, leaving TTOT at %s.", prefix, to)
	}
	return prefix + "."
}

func storedSequencePosition(calculation *models.CdmCalculation) *int {
	if calculation == nil || calculation.SequencePosition == nil {
		return nil
	}
	position := *calculation.SequencePosition
	return &position
}

func storedTaxiMinutes(calculation *models.CdmCalculation) *int {
	if calculation == nil || calculation.TaxiMinutes == nil {
		return nil
	}
	minutes := *calculation.TaxiMinutes
	return &minutes
}

func storedTaxiRunway(calculation *models.CdmCalculation, fallback string) string {
	if calculation != nil && calculation.TaxiRunway != nil {
		return valueOrEmpty(calculation.TaxiRunway)
	}
	return fallback
}

func sortPersistedRows(rows []sequenceSnapshotRow, now time.Time) {
	sort.SliceStable(rows, func(i, j int) bool {
		leftPosition := rows[i].response.Position
		rightPosition := rows[j].response.Position
		switch {
		case leftPosition != nil && rightPosition != nil && *leftPosition != *rightPosition:
			return *leftPosition < *rightPosition
		case leftPosition != nil && rightPosition == nil:
			return true
		case leftPosition == nil && rightPosition != nil:
			return false
		}
		if cmp := compareClockForSort(rows[i].sortTTOT, rows[j].sortTTOT, now); cmp != 0 {
			return cmp < 0
		}
		if cmp := compareClockForSort(rows[i].sortBase, rows[j].sortBase, now); cmp != 0 {
			return cmp < 0
		}
		return rows[i].response.Callsign < rows[j].response.Callsign
	})

	position := 0
	for i := range rows {
		if rows[i].response.Position == nil && rows[i].sortTTOT != "" {
			position++
			positionValue := position
			rows[i].response.Position = &positionValue
			continue
		}
		if rows[i].response.Position != nil && *rows[i].response.Position > position {
			position = *rows[i].response.Position
		}
	}
}

func assignSequencePositions(rows []sequenceSnapshotRow, now time.Time) {
	sequenced := make([]*sequenceSnapshotRow, 0, len(rows))
	unsequenced := make([]*sequenceSnapshotRow, 0, len(rows))
	for i := range rows {
		if rows[i].sortTTOT == "" {
			unsequenced = append(unsequenced, &rows[i])
			continue
		}
		sequenced = append(sequenced, &rows[i])
	}

	sort.SliceStable(sequenced, func(i, j int) bool {
		left := sequenced[i]
		right := sequenced[j]
		if cmp := compareClockForSort(left.sortTTOT, right.sortTTOT, now); cmp != 0 {
			return cmp < 0
		}
		if cmp := compareClockForSort(left.sortBase, right.sortBase, now); cmp != 0 {
			return cmp < 0
		}
		return left.response.Callsign < right.response.Callsign
	})

	for i, row := range sequenced {
		position := i + 1
		row.response.Position = &position
	}

	sort.SliceStable(unsequenced, func(i, j int) bool {
		left := unsequenced[i]
		right := unsequenced[j]
		if cmp := compareClockForSort(left.sortBase, right.sortBase, now); cmp != 0 {
			return cmp < 0
		}
		return left.response.Callsign < right.response.Callsign
	})

	ordered := make([]sequenceSnapshotRow, 0, len(rows))
	for _, row := range sequenced {
		ordered = append(ordered, *row)
	}
	for _, row := range unsequenced {
		ordered = append(ordered, *row)
	}
	copy(rows, ordered)
}

func cloneSequenceData(data *models.CdmData) *models.CdmData {
	if data == nil {
		return (&models.CdmData{}).Normalize()
	}
	return data.Clone().Normalize()
}

func truncateSequenceClock(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 4 {
		return value[:4]
	}
	return value
}

func (a *WebAPI) authenticate(w http.ResponseWriter, r *http.Request) (shared.AuthenticatedUser, bool) {
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader == "" {
		writeJSONError(w, http.StatusUnauthorized, "missing authorization header")
		return shared.AuthenticatedUser{}, false
	}

	token := strings.TrimSpace(strings.TrimPrefix(authHeader, "Bearer"))
	if token == authHeader || token == "" {
		writeJSONError(w, http.StatusUnauthorized, "invalid authorization header")
		return shared.AuthenticatedUser{}, false
	}

	user, err := a.authenticationService.Validate(token)
	if err != nil {
		writeJSONError(w, http.StatusUnauthorized, "invalid token")
		return shared.AuthenticatedUser{}, false
	}

	return user, true
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
