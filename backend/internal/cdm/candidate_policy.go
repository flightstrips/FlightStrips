package cdm

import (
	"context"
	"fmt"
	"strings"
	"time"

	"FlightStrips/internal/models"
)

// PlanSequence executes the production sequence policy against isolated input
// copies. Results are buffered; callers commit the complete result atomically.
// No repository, hub, timer or provider is constructed here.
func PlanSequence(ctx context.Context, session int32, strips []*models.Strip, config *CdmAirportConfig, now time.Time) (map[string]*models.CdmData, error) {
	if config == nil || now.IsZero() {
		return nil, fmt.Errorf("sequence requires accepted configuration and time")
	}
	buffer := &sequenceBuffer{results: map[string]*models.CdmData{}}
	copies := make([]*models.Strip, 0, len(strips))
	for _, strip := range strips {
		if strip == nil {
			continue
		}
		copy := *strip
		copy.CdmData = strip.CdmData.Clone()
		copies = append(copies, &copy)
	}
	service := &SequenceService{stripRepo: buffer}
	if err := service.sequenceSnapshot(ctx, session, copies, config.Clone(), now.UTC(), false); err != nil {
		return nil, err
	}
	return buffer.results, nil
}

// CandidateAction is a closed operational policy operation. Transport adapters
// translate their typed command to this value; it is never serialized as data.
type CandidateAction struct {
	Kind, Value, Position, Role string
}

// PlanAction reuses production clock, confirmation, clamping and recalculation
// helpers. External writes are planned separately, after local acceptance.
func PlanAction(strip *models.Strip, config *CdmAirportConfig, action CandidateAction, now time.Time) (*models.CdmData, error) {
	if strip == nil || config == nil || now.IsZero() {
		return nil, fmt.Errorf("CDM action input unavailable")
	}
	d := strip.CdmData.Clone()
	v := strings.TrimSpace(action.Value)
	policy := &ActionService{}
	confirm := func(value string) {
		applyConfirmedTobtUpdate(d, value, action.Position, action.Role)
		applyTobtRecalculationPolicy(d, value)
		if d.RecalculationMode == models.CdmRecalculationRequired && d.HasManualCtot() {
			earliest := addMinutes(toHHMMSS(value), float64(resolveTaxiMinutesForStrip(strip, config)+deiceTypeToMinutes(config, valueOrEmpty(d.DeIce))))
			if minutesBetween(toHHMMSS(valueOrEmpty(d.Ctot)), earliest) > 0 {
				d.Ctot, d.CtotSource = nil, nil
			}
		}
	}
	switch action.Kind {
	case "tobt", "clx-tobt", "pushback":
		if action.Kind == "pushback" {
			v = now.UTC().Format("1504")
		}
		if !isValidHHMM(v) {
			return nil, fmt.Errorf("valid TOBT required")
		}
		confirm(v)
		if action.Kind == "pushback" && d.NeedsLocalRecalculation() {
			d.PushbackRecalculate = true
		}
	case "eobt":
		if v != "" && !isValidHHMM(v) {
			return nil, fmt.Errorf("valid EOBT required")
		}
		d = policy.prepareEobtUpdate(strip.Session, d, v, now.UTC(), true).updated
	case "logon":
		d = policy.PrepareEuroscopeLogonSync(d, v, now.UTC())
	case "clearance-tobt":
		if value, adjust := clearanceTobt(d, now); adjust {
			applyAutoSyncedTobtUpdate(d, value)
			applyTobtRecalculationPolicy(d, value)
		}
	case "deice":
		if !isValidDeiceType(v) {
			return nil, fmt.Errorf("invalid deice type")
		}
		if valueOrEmpty(d.DeIce) != v {
			d.DeIce = stringPointerIfPresent(v)
			d.MarkLocalRecalculationPending()
		}
	case "asrt":
		if v != "" && !isValidHHMM(v) {
			return nil, fmt.Errorf("valid ASRT required")
		}
		d.Asrt = stringPointerIfPresent(v)
	case "tsac":
		if v != "" && !isValidHHMM(v) {
			return nil, fmt.Errorf("valid TSAC required")
		}
		d.Tsac = stringPointerIfPresent(v)
	case "ctot":
		if !isValidHHMM(v) {
			return nil, fmt.Errorf("valid CTOT required")
		}
		if valueOrEmpty(d.Ctot) != v || !d.HasManualCtot() {
			d.Ctot = &v
			source := models.CtotSourceManual
			d.CtotSource = &source
			d.MarkLocalRecalculationPending()
		}
	case "remove-ctot":
		if d.HasManualCtot() {
			d.Ctot, d.CtotSource = nil, nil
			d.MarkLocalRecalculationPending()
		}
	case "ready":
		if value, change := readyRequestTobt(d, now); change {
			confirm(value)
		}
		clock := now.UTC().Format("1504")
		if valueOrEmpty(d.Asrt) == "" {
			d.Asrt = &clock
		}
		status := "REA"
		d.Status = &status
		if current := normalizeCalculationClock(valueOrEmpty(d.EffectiveEobt())); current == "" || !isAfterOrEqual(clock, current) {
			d.Eobt = &clock
		}
	case "ground-state":
		clock := now.UTC().Format("1504")
		if groundStateAllowsAsat(v) && valueOrEmpty(d.Asat) == "" {
			d.Asat = &clock
		}
		if !groundStateAllowsAsat(v) && valueOrEmpty(d.Asat) != "" {
			d.Asat = nil
			d.MarkLocalRecalculationPending()
		}
		if v == "PUSH" && valueOrEmpty(d.Aobt) == "" {
			d.Aobt = &clock
		}
	case "aobt":
		if valueOrEmpty(d.Aobt) == "" {
			clock := now.UTC().Format("1504")
			d.Aobt = &clock
		}
	case "atot":
		if valueOrEmpty(d.Atot) == "" {
			clock := now.UTC().Format("1504")
			d.Atot = &clock
		}
	case "better-tobt":
		status := "REQTOBT/" + now.UTC().Format("1504") + "/ATC"
		d.Status = &status
	default:
		return nil, fmt.Errorf("unsupported CDM action %q", action.Kind)
	}
	return d.Normalize(), nil
}

// MergeViffFlight retains the production master rule: only CTOT and REQTOBT
// enter local calculation; controller/manual actual times stay authoritative.
func MergeViffFlight(data *models.CdmData, row IFPSData) *models.CdmData {
	d := data.Clone()
	ctot, source := effectiveIfpsCtotAndSource(row)
	changed := valueOrEmpty(d.Ctot) != ctot
	if ctot != "" {
		d.Ctot, d.CtotSource = &ctot, &source
		d.MostPenalizingAirspace = stringPointerIfPresent(row.MostPenalizingAirspace)
		d.EcfmpID = stringPointerIfPresent(row.CDMData.Reason)
	} else if !d.HasManualCtot() {
		d.Ctot, d.CtotSource, d.MostPenalizingAirspace, d.EcfmpID = nil, nil, nil, nil
	}
	request := truncateCDMClockValue(strings.TrimSpace(row.CDMData.ReqTOBT))
	requestSource := strings.ToUpper(strings.TrimSpace(row.CDMData.ReqTOBTType))
	if requestSource == "" {
		requestSource = "VIFF"
	}
	if isValidHHMM(request) && (valueOrEmpty(d.Tobt) != request || valueOrEmpty(d.TobtSetBy) != "vIFF" || valueOrEmpty(d.TobtConfirmedBy) != requestSource) {
		d.Tobt = &request
		by := "vIFF"
		d.TobtSetBy = &by
		d.TobtConfirmedBy = &requestSource
		d.TobtAutoSynced = false
		d.TobtManuallyConfirmed = true
		d.ViffRequestSyncPending = true
		applyTobtRecalculationPolicy(d, request)
	} else if changed {
		d.MarkLocalRecalculationPending()
	}
	return d
}

// ViffProposal builds the same external proposal as the production master.
func ViffProposal(strip *models.Strip) (SetCdmDataParams, bool, bool) {
	if strip == nil {
		return SetCdmDataParams{}, false, false
	}
	state, ok := buildViffPushState(strip.Callsign, strip, strip.CdmData)
	return state.Params, state.Suspend, ok
}

func ViffEnabledSession(name string) bool { return isViffEnabledSession(name) }

func ViffNeedsExport(data *models.CdmData, row IFPSData) bool {
	return masterFlightNeedsExport(data, row)
}

func CandidateTaxiMinutes(strip *models.Strip, config *CdmAirportConfig) int {
	return resolveTaxiMinutesForStrip(strip, config)
}

// NormalizeSessionEobt applies the production periodic clamp only when needed;
// a poll must not turn an unchanged earlier-TOBT request into a forced requeue.
func NormalizeSessionEobt(data *models.CdmData, now time.Time) *models.CdmData {
	d := data.Clone()
	if d.TobtAutoAdjusted {
		return d
	}
	policy := &ActionService{}
	value, clamped := policy.normalizeMasterEobtValue(0, valueOrEmpty(d.Eobt), now)
	if !clamped {
		return d
	}
	previous := valueOrEmpty(d.Eobt)
	setEobtCapReasonMarker(d, true, false)
	d.Eobt = &value
	if shouldSyncEobtToTobt(value, now) && !hasProtectedConfirmedTobt(d, previous) {
		applyAutoSyncedTobtUpdate(d, value)
	}
	d.MarkLocalRecalculationPending()
	return d
}

type sequenceBuffer struct{ results map[string]*models.CdmData }

func (b *sequenceBuffer) ListByOrigin(context.Context, int32, string) ([]*models.Strip, error) {
	return nil, fmt.Errorf("sequence input must be supplied explicitly")
}
func (b *sequenceBuffer) SetCdmData(_ context.Context, _ int32, callsign string, data *models.CdmData) (int64, error) {
	b.results[callsign] = data.Clone()
	return 1, nil
}
