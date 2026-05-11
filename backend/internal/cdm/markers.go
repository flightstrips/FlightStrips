package cdm

import (
	"FlightStrips/internal/models"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
)

type storedSequenceRow struct {
	strip      *models.Strip
	data       *models.CdmData
	baseTime   string
	baseSource string
	ttot       string
}

func buildStoredSequenceMarkerUpdates(strips []*models.Strip, isMaster bool, anchor time.Time) map[string]*models.CdmData {
	rows := make([]storedSequenceRow, 0, len(strips))
	for _, strip := range strips {
		if strip == nil || shouldSkipStrip(strip) {
			continue
		}

		data := cloneSequenceData(strip.CdmData)
		baseTime, baseSource := remoteSequenceBase(data)
		rows = append(rows, storedSequenceRow{
			strip:      strip,
			data:       data,
			baseTime:   baseTime,
			baseSource: baseSource,
			ttot:       normalizeCalculationClock(valueOrEmpty(data.EffectiveTtot())),
		})
	}

	sortStoredSequenceRows(rows, anchor)

	updates := make(map[string]*models.CdmData, len(rows))
	var previous *storedSequenceRow
	position := 0
	for _, row := range rows {
		updated := row.data.Clone().Normalize()
		if row.ttot != "" {
			position++
		}
		applyStoredSequenceMarkers(updated, row, isMaster, position, previous)
		if row.strip == nil || reflect.DeepEqual(row.strip.CdmData, updated) {
			continue
		}
		updates[row.strip.Callsign] = updated
		if row.ttot != "" {
			rowCopy := row
			previous = &rowCopy
		}
	}

	return updates
}

func sortStoredSequenceRows(rows []storedSequenceRow, anchor time.Time) {
	sort.SliceStable(rows, func(i, j int) bool {
		left := rows[i]
		right := rows[j]

		if left.ttot != "" || right.ttot != "" {
			switch {
			case left.ttot == "" && right.ttot != "":
				return false
			case left.ttot != "" && right.ttot == "":
				return true
			}
			if cmp := compareClockForSort(left.ttot, right.ttot, anchor); cmp != 0 {
				return cmp < 0
			}
		}

		if cmp := compareClockForSort(left.baseTime, right.baseTime, anchor); cmp != 0 {
			return cmp < 0
		}

		return left.strip.Callsign < right.strip.Callsign
	})
}

func applyStoredSequenceMarkers(data *models.CdmData, row storedSequenceRow, isMaster bool, position int, previous *storedSequenceRow) {
	if data == nil {
		return
	}

	calculation := data.Calculation.Clone()
	if calculation == nil {
		calculation = &models.CdmCalculation{}
	}

	calculation.BaseTime = stringPointerIfPresent(row.baseTime)
	calculation.BaseSource = stringPointerIfPresent(row.baseSource)
	calculation.TaxiMinutes = deriveStoredTaxiMinutes(data)
	calculation.TaxiRunway = stringPointerIfPresent(valueOrEmpty(row.strip.Runway))
	calculation.SequencePosition = nil
	calculation.LeaderCallsign = nil
	calculation.LeaderTtot = nil
	if row.ttot != "" {
		calculation.SequencePosition = intPointerIfPositive(position)
	}
	if previous != nil {
		calculation.LeaderCallsign = stringPointerIfPresent(previous.strip.Callsign)
		calculation.LeaderTtot = stringPointerIfPresent(previous.ttot)
	}
	calculation.ReasonMarkers = buildStoredReasonMarkers(row, isMaster, calculation.TaxiMinutes, previous)
	data.Calculation = calculation
	data.Normalize()
}

func buildStoredReasonMarkers(row storedSequenceRow, isMaster bool, taxiMinutes *int, previous *storedSequenceRow) []models.CdmReasonMarker {
	reasons := make([]models.CdmReasonMarker, 0, 6)
	sourceMessage := "Stored slot comes from synced CDM data for this session."
	if isMaster {
		sourceMessage = "Stored slot comes from the session's master-sequenced CDM data."
	}
	reasons = append(reasons, marker("slot_source", sourceMessage, ""))

	if row.baseTime != "" {
		reasons = append(reasons, marker("base_time", buildStoredBaseMessage(row.baseSource, row.baseTime), ""))
	}
	if taxiMinutes != nil && *taxiMinutes > 0 {
		reasons = append(reasons, marker("taxi_time", buildStoredTaxiMessage(*taxiMinutes, valueOrEmpty(row.strip.Runway)), ""))
	}
	if confirmedBy := valueOrEmpty(row.data.TobtConfirmedBy); confirmedBy != "" {
		reasons = append(reasons, marker("tobt_confirmation", "TOBT is confirmed by "+confirmedBy+".", ""))
	}
	if ctot := truncateStoredTime(valueOrEmpty(row.data.EffectiveCtot())); ctot != "" {
		reasons = append(reasons, marker("ctot", "Stored CTOT is "+ctot+".", ""))
	}
	if invalidReason := remoteSequenceInvalidReason(row.data); invalidReason != "" {
		reasons = append(reasons, marker(buildInvalidReason(row.data).Kind, buildInvalidReason(row.data).Message, ""))
	}
	reasons = append(reasons, buildStoredOrderMarker(row, previous))
	return reasons
}

func setCalculationReasonMarkers(data *models.CdmData, markers []models.CdmReasonMarker) {
	if data == nil {
		return
	}

	calculation := data.Calculation.Clone()
	if calculation == nil {
		calculation = &models.CdmCalculation{}
	}

	if len(markers) == 0 {
		calculation.ReasonMarkers = nil
	} else {
		cloned := make([]models.CdmReasonMarker, len(markers))
		for index, marker := range markers {
			cloned[index] = models.CdmReasonMarker{
				Kind:                   marker.Kind,
				Message:                marker.Message,
				AgainstCallsign:        stringPointerClone(marker.AgainstCallsign),
				AgainstRunway:          stringPointerClone(marker.AgainstRunway),
				AgainstTtot:            stringPointerClone(marker.AgainstTtot),
				FromTtot:               stringPointerClone(marker.FromTtot),
				ToTtot:                 stringPointerClone(marker.ToTtot),
				RequiredSpacingMinutes: floatPointerClone(marker.RequiredSpacingMinutes),
			}
		}
		calculation.ReasonMarkers = cloned
	}

	data.Calculation = calculation
	data.Normalize()
}

func movementReasonMarkersFromTrace(trace []calculationTraceEntry) []models.CdmReasonMarker {
	if len(trace) == 0 {
		return nil
	}

	compacted := compactCalculationTrace(trace)
	if len(compacted) == 0 {
		return nil
	}
	return []models.CdmReasonMarker{reasonMarkerFromTrace(compacted[len(compacted)-1])}
}

func invalidReasonMarker(data *models.CdmData) []models.CdmReasonMarker {
	reason := buildInvalidReason(data)
	if reason.Kind == "unsequenced" {
		return nil
	}
	return []models.CdmReasonMarker{marker(reason.Kind, reason.Message, "")}
}

func compactCalculationTrace(trace []calculationTraceEntry) []calculationTraceEntry {
	compacted := make([]calculationTraceEntry, 0, len(trace))
	for _, entry := range trace {
		if shouldSkipTraceEntry(entry) {
			continue
		}
		if len(compacted) == 0 {
			compacted = append(compacted, normalizeTraceEntryKind(entry))
			continue
		}

		lastIndex := len(compacted) - 1
		if shouldMergeTraceEntries(compacted[lastIndex], entry) {
			compacted[lastIndex] = mergeTraceEntries(compacted[lastIndex], entry)
			continue
		}

		compacted = append(compacted, normalizeTraceEntryKind(entry))
	}
	return compacted
}

func shouldSkipTraceEntry(entry calculationTraceEntry) bool {
	from := normalizeCalculationClock(entry.FromTtot)
	to := normalizeCalculationClock(entry.ToTtot)
	return from != "" && to != "" && from == to
}

func shouldMergeTraceEntries(previous calculationTraceEntry, current calculationTraceEntry) bool {
	if !strings.EqualFold(strings.TrimSpace(previous.AgainstCallsign), strings.TrimSpace(current.AgainstCallsign)) {
		return false
	}
	if traceReasonFamily(previous.Kind) != traceReasonFamily(current.Kind) {
		return false
	}
	if traceReasonFamily(previous.Kind) == "runway_spacing" {
		return strings.EqualFold(strings.TrimSpace(previous.AgainstRunway), strings.TrimSpace(current.AgainstRunway))
	}
	return strings.EqualFold(strings.TrimSpace(previous.Kind), strings.TrimSpace(current.Kind))
}

func mergeTraceEntries(previous calculationTraceEntry, current calculationTraceEntry) calculationTraceEntry {
	merged := normalizeTraceEntryKind(previous)
	current = normalizeTraceEntryKind(current)
	merged.Kind = current.Kind
	if merged.AgainstCallsign == "" {
		merged.AgainstCallsign = current.AgainstCallsign
	}
	if merged.AgainstRunway == "" {
		merged.AgainstRunway = current.AgainstRunway
	}
	if merged.AgainstTtot == "" {
		merged.AgainstTtot = current.AgainstTtot
	}
	if merged.AgainstSid == "" {
		merged.AgainstSid = current.AgainstSid
	}
	if merged.FromTtot == "" {
		merged.FromTtot = current.FromTtot
	}
	if current.ToTtot != "" {
		merged.ToTtot = current.ToTtot
	}
	if current.RequiredSpacingMinutes > merged.RequiredSpacingMinutes {
		merged.RequiredSpacingMinutes = current.RequiredSpacingMinutes
	}
	return merged
}

func normalizeTraceEntryKind(entry calculationTraceEntry) calculationTraceEntry {
	if traceReasonFamily(entry.Kind) == "runway_spacing" {
		entry.Kind = "runway_spacing"
	}
	return entry
}

func traceReasonFamily(kind string) string {
	switch kind {
	case "runway_slot_collision", "runway_rate_window", "runway_spacing":
		return "runway_spacing"
	default:
		return kind
	}
}

func reasonMarkerFromTrace(entry calculationTraceEntry) models.CdmReasonMarker {
	marker := models.CdmReasonMarker{
		Kind:            normalizeTraceEntryKind(entry).Kind,
		AgainstCallsign: stringPointerIfPresent(strings.TrimSpace(entry.AgainstCallsign)),
		AgainstRunway:   stringPointerIfPresent(strings.TrimSpace(entry.AgainstRunway)),
		AgainstTtot:     stringPointerIfPresent(normalizeCalculationClock(entry.AgainstTtot)),
		FromTtot:        stringPointerIfPresent(normalizeCalculationClock(entry.FromTtot)),
		ToTtot:          stringPointerIfPresent(normalizeCalculationClock(entry.ToTtot)),
	}
	if entry.RequiredSpacingMinutes > 0 {
		spacing := entry.RequiredSpacingMinutes
		marker.RequiredSpacingMinutes = &spacing
	}
	return marker
}

func stringPointerClone(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func floatPointerClone(value *float64) *float64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func buildStoredBaseMessage(baseSource string, baseTime string) string {
	sourceLabel := baseSource
	switch baseSource {
	case models.CdmCalculationBaseTobt:
		sourceLabel = "TOBT"
	case models.CdmCalculationBaseReqTobt:
		sourceLabel = "REQ TOBT"
	case models.CdmCalculationBaseEobt:
		sourceLabel = "EOBT"
	}
	return sourceLabel + " " + truncateStoredTime(baseTime) + " is the stored sequencing base."
}

func buildStoredTaxiMessage(minutes int, runway string) string {
	message := "Stored taxi time is " + strconv.Itoa(minutes) + " min."
	if runway != "" {
		message = "Stored taxi time is " + strconv.Itoa(minutes) + " min for runway " + runway + "."
	}
	return message
}

func buildStoredOrderMarker(row storedSequenceRow, previous *storedSequenceRow) models.CdmReasonMarker {
	if row.ttot == "" {
		return marker("stored_sequence_order", "No stored TTOT is available yet, so the flight is currently unsequenced.", "")
	}
	if previous == nil || previous.ttot == "" {
		return marker("stored_sequence_order", "This flight is first because it has the earliest stored TTOT in the session ("+truncateStoredTime(row.ttot)+").", "")
	}
	return marker("stored_sequence_order", "This flight follows "+previous.strip.Callsign+" because its stored TTOT "+truncateStoredTime(row.ttot)+" is later than "+previous.strip.Callsign+"'s "+truncateStoredTime(previous.ttot)+".", previous.strip.Callsign)
}

func deriveStoredTaxiMinutes(data *models.CdmData) *int {
	if data == nil {
		return nil
	}
	if calculation := data.EffectiveCalculation(); calculation != nil && calculation.TaxiMinutes != nil && *calculation.TaxiMinutes > 0 {
		return cloneStoredInt(calculation.TaxiMinutes)
	}

	tsat := normalizeCalculationClock(valueOrEmpty(data.EffectiveTsat()))
	ttot := normalizeCalculationClock(valueOrEmpty(data.EffectiveTtot()))
	if tsat == "" || ttot == "" {
		return nil
	}

	minutes := int(math.Round(minutesBetween(tsat, ttot)))
	if minutes <= 0 {
		return nil
	}
	return &minutes
}

func cloneStoredInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func marker(kind string, message string, against string) models.CdmReasonMarker {
	result := models.CdmReasonMarker{
		Kind:    kind,
		Message: message,
	}
	if against != "" {
		result.AgainstCallsign = stringPointerIfPresent(against)
	}
	return result
}

func truncateStoredTime(value string) string {
	value = normalizeCalculationClock(value)
	if len(value) > 4 {
		return value[:4]
	}
	return value
}
