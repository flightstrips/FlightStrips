package shared

import (
	internalModels "FlightStrips/internal/models"
	"context"
	"slices"
)

type syncStateKey struct{}

// SyncState carries preloaded sync data so EuroScope sync processing can avoid
// repeating the same session/controller/strip lookups for every item.
type SyncState struct {
	Session             *internalModels.Session
	ExistingControllers map[string]*internalModels.Controller
	ExistingStrips      map[string]*internalModels.Strip
	SectorOwners        map[string]*internalModels.SectorOwner
	GndOnline           bool
	ChangedControllers  int
	ChangedStrips       int
	DBOperations        int
	RouteRecalcStrips   map[string]struct{}
	BayUpdates          map[string]string
	PdcValidationStrips map[string]struct{}
	StripUpdates        map[string]struct{}
	SquawkValidation    bool
	LandingValidation   bool
	CdmRecalculation    bool
}

func WithSyncState(ctx context.Context, state *SyncState) context.Context {
	return context.WithValue(ctx, syncStateKey{}, state)
}

func GetSyncState(ctx context.Context) *SyncState {
	state, _ := ctx.Value(syncStateKey{}).(*SyncState)
	return state
}

func (s *SyncState) AddDBOperations(count int) {
	if s == nil {
		return
	}
	s.DBOperations += count
}

func (s *SyncState) MarkRouteRecalc(callsign string) {
	if s == nil || callsign == "" {
		return
	}
	if s.RouteRecalcStrips == nil {
		s.RouteRecalcStrips = make(map[string]struct{})
	}
	s.RouteRecalcStrips[callsign] = struct{}{}
}

func (s *SyncState) MarkBayUpdate(callsign string, bay string) {
	if s == nil || callsign == "" {
		return
	}
	if s.BayUpdates == nil {
		s.BayUpdates = make(map[string]string)
	}
	s.BayUpdates[callsign] = bay
}

func (s *SyncState) MarkPdcValidation(callsign string) {
	if s == nil || callsign == "" {
		return
	}
	if s.PdcValidationStrips == nil {
		s.PdcValidationStrips = make(map[string]struct{})
	}
	s.PdcValidationStrips[callsign] = struct{}{}
}

func (s *SyncState) MarkStripUpdate(callsign string) {
	if s == nil || callsign == "" {
		return
	}
	if s.StripUpdates == nil {
		s.StripUpdates = make(map[string]struct{})
	}
	s.StripUpdates[callsign] = struct{}{}
}

func (s *SyncState) SortedRouteRecalcStrips() []string {
	if s == nil {
		return nil
	}
	return sortedSyncStateKeys(s.RouteRecalcStrips)
}

func (s *SyncState) SortedPdcValidationStrips() []string {
	if s == nil {
		return nil
	}
	return sortedSyncStateKeys(s.PdcValidationStrips)
}

func (s *SyncState) SortedStripUpdates() []string {
	if s == nil {
		return nil
	}
	return sortedSyncStateKeys(s.StripUpdates)
}

func (s *SyncState) SortedBayUpdateCallsigns() []string {
	if s == nil {
		return nil
	}
	callsigns := make([]string, 0, len(s.BayUpdates))
	for callsign := range s.BayUpdates {
		callsigns = append(callsigns, callsign)
	}
	slices.Sort(callsigns)
	return callsigns
}

func sortedSyncStateKeys(values map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
