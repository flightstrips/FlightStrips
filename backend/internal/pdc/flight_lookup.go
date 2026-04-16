package pdc

import (
	"FlightStrips/internal/models"
	"FlightStrips/internal/pilot"
	"context"
	"errors"
	"fmt"
	"time"
)

// FlightLookupAdapter bridges the PDC service to the pilot.FlightLookup interface.
type FlightLookupAdapter struct {
	pdcService  *Service
	sessionRepo interface {
		GetByID(ctx context.Context, id int32) (*models.Session, error)
	}
}

func NewFlightLookupAdapter(pdcService *Service, sessionRepo interface {
	GetByID(ctx context.Context, id int32) (*models.Session, error)
}) *FlightLookupAdapter {
	return &FlightLookupAdapter{pdcService: pdcService, sessionRepo: sessionRepo}
}

func (a *FlightLookupAdapter) GetFlightInfo(ctx context.Context, callsign string) (*pilot.FlightInfo, error) {
	match, err := a.pdcService.FindWebStripByCallsign(ctx, callsign)
	if err != nil {
		switch {
		case errors.Is(err, ErrWebStripNotFound):
			return nil, pilot.ErrFlightNotFound
		case errors.Is(err, ErrWebAmbiguousCallsign):
			return nil, pilot.ErrAmbiguousCallsign
		default:
			return nil, fmt.Errorf("find strip by callsign: %w", err)
		}
	}

	session, err := a.sessionRepo.GetByID(ctx, match.SessionID)
	if err != nil {
		return nil, fmt.Errorf("get session for flight: %w", err)
	}

	strip := match.Strip
	isDeparture := strip.Origin == session.Airport
	pdcAvailable := isDeparture && !strip.Cleared

	// REQUESTED_WITH_FAULTS is shown to pilots as REQUESTED (same as existing pdc web API)
	presentedState := strip.PdcState
	if presentedState == "REQUESTED_WITH_FAULTS" {
		presentedState = "REQUESTED"
	}

	info := &pilot.FlightInfo{
		Callsign:               strip.Callsign,
		Origin:                 strip.Origin,
		Destination:            strip.Destination,
		IsDeparture:            isDeparture,
		Cleared:                strip.Cleared,
		PdcAvailable:           pdcAvailable,
		PdcCanSubmit:           WebPDCCanSubmit(strip.PdcState),
		PdcState:               presentedState,
		PdcRequiresPilotAction: presentedState == "CLEARED",
	}

	if strip.PdcRequestRemarks != nil && *strip.PdcRequestRemarks != "" {
		info.PdcRequestRemarks = strip.PdcRequestRemarks
	}

	if strip.PdcData != nil && strip.PdcData.Web != nil {
		info.PdcClearanceText = strip.PdcData.Web.ClearanceText
		if strip.PdcData.Web.PilotAcknowledgedAt != nil {
			formatted := strip.PdcData.Web.PilotAcknowledgedAt.UTC().Format(time.RFC3339)
			info.PdcAcknowledgedAt = &formatted
		}
	}

	return info, nil
}
