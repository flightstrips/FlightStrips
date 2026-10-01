package services

import (
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	"context"
	"strings"
)

func ensureMessageStateStripIndex(state *shared.WebsocketMessageState) {
	if state == nil || state.ExistingStrips != nil {
		return
	}
	state.ExistingStrips = make(map[string]*internalModels.Strip, len(state.StripList))
	for _, strip := range state.StripList {
		if strip == nil {
			continue
		}
		state.ExistingStrips[strings.ToUpper(strings.TrimSpace(strip.Callsign))] = strip
	}
}

func normalizedCallsignKey(callsign string) string {
	return strings.ToUpper(strings.TrimSpace(callsign))
}

func updateCachedStripStand(ctx context.Context, callsign, stand string, incrementVersion bool) {
	key := normalizedCallsignKey(callsign)
	setStand := func(strip *internalModels.Strip) {
		if strip == nil {
			return
		}
		value := stand
		strip.Stand = &value
		if incrementVersion {
			strip.Version++
		}
	}
	if syncState := shared.GetSyncState(ctx); syncState != nil && syncState.ExistingStrips != nil {
		strip := syncState.ExistingStrips[callsign]
		if strip == nil {
			strip = syncState.ExistingStrips[key]
		}
		setStand(strip)
	}
	if state := shared.GetWebsocketMessageState(ctx); state != nil {
		ensureMessageStateStripIndex(state)
		setStand(state.ExistingStrips[key])
	}
}
