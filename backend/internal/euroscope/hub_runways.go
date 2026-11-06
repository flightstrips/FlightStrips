package euroscope

import (
	esEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"FlightStrips/pkg/models"
	"fmt"
	"slices"
	"strings"
)

type clientRunwayState struct {
	Client             *Client
	Session            int32
	CID                string
	Callsign           string
	DepartureRunways   []string
	ArrivalRunways     []string
	DepartureMismatch  bool
	ArrivalMismatch    bool
	LastAlertSignature string
}

type runwayClientEvaluation struct {
	Target            *Client
	CID               string
	DepartureMismatch bool
	ArrivalMismatch   bool
	Changed           bool
	Alert             *esEvents.RunwayMismatchAlertEvent
}

func normalizeRunwaySlice(runways []string) []string {
	if runways == nil {
		return []string{}
	}
	return slices.Clone(runways)
}

func cloneRunwayStatus(status map[string]string) map[string]string {
	if status == nil {
		return nil
	}
	cloned := make(map[string]string, len(status))
	for key, value := range status {
		cloned[key] = value
	}
	return cloned
}

func normalizeActiveRunways(active models.ActiveRunways) models.ActiveRunways {
	return models.ActiveRunways{
		DepartureRunways: normalizeRunwaySlice(active.DepartureRunways),
		ArrivalRunways:   normalizeRunwaySlice(active.ArrivalRunways),
		RunwayStatus:     cloneRunwayStatus(active.RunwayStatus),
	}
}

func buildFrontendRunwayConfiguration(active models.ActiveRunways, departureMismatch, arrivalMismatch bool) frontendEvents.RunwayConfiguration {
	active = normalizeActiveRunways(active)
	return frontendEvents.RunwayConfiguration{
		Departure:         active.DepartureRunways,
		Arrival:           active.ArrivalRunways,
		RunwayStatus:      active.RunwayStatus,
		DepartureMismatch: departureMismatch,
		ArrivalMismatch:   arrivalMismatch,
	}
}

func runwayStateKey(session int32, cid string) string {
	return fmt.Sprintf("%d:%s", session, cid)
}

func buildRunwayMismatchAlert(master, current models.ActiveRunways) esEvents.RunwayMismatchAlertEvent {
	return esEvents.RunwayMismatchAlertEvent{
		ExpectedDeparture: normalizeRunwaySlice(master.DepartureRunways),
		ExpectedArrival:   normalizeRunwaySlice(master.ArrivalRunways),
		CurrentDeparture:  normalizeRunwaySlice(current.DepartureRunways),
		CurrentArrival:    normalizeRunwaySlice(current.ArrivalRunways),
	}
}

func runwayMismatchSignature(master, current models.ActiveRunways) string {
	serialize := func(values []string) string {
		return strings.Join(normalizeRunwaySlice(values), ",")
	}

	return strings.Join([]string{
		serialize(master.DepartureRunways),
		serialize(master.ArrivalRunways),
		serialize(current.DepartureRunways),
		serialize(current.ArrivalRunways),
	}, "|")
}

func (hub *Hub) clearClientRunwayState(session int32, cid string) {
	if cid == "" {
		return
	}

	hub.runwayStateMu.Lock()
	delete(hub.runwayStates, runwayStateKey(session, cid))
	hub.runwayStateMu.Unlock()
}

func (hub *Hub) GetRunwayMismatchStatus(session int32, cid string) (bool, bool) {
	if cid == "" {
		return false, false
	}

	hub.runwayStateMu.RLock()
	defer hub.runwayStateMu.RUnlock()

	state, ok := hub.runwayStates[runwayStateKey(session, cid)]
	if ok {
		return state.DepartureMismatch, state.ArrivalMismatch
	}
	var departure, arrival bool
	for _, candidate := range hub.runwayStates {
		if candidate.Session == session && candidate.CID == cid {
			departure = departure || candidate.DepartureMismatch
			arrival = arrival || candidate.ArrivalMismatch
		}
	}
	return departure, arrival
}

func (hub *Hub) evaluateClientRunwayState(session int32, cid, callsign string, current, master models.ActiveRunways, isMaster bool) runwayClientEvaluation {
	return hub.evaluateRunwayState(session, cid, callsign, nil, current, master, isMaster)
}
func (hub *Hub) evaluateMembershipRunwayState(client *Client, current, master models.ActiveRunways, isMaster bool) runwayClientEvaluation {
	return hub.evaluateRunwayState(client.session, client.GetCid(), client.GetCallsign(), client, current, master, isMaster)
}
func (hub *Hub) evaluateRunwayState(session int32, cid, callsign string, client *Client, current, master models.ActiveRunways, isMaster bool) runwayClientEvaluation {
	current = normalizeActiveRunways(current)
	master = normalizeActiveRunways(master)

	evaluation := runwayClientEvaluation{CID: cid, Target: client}
	if cid == "" {
		return evaluation
	}

	hub.runwayStateMu.Lock()
	defer hub.runwayStateMu.Unlock()

	key := runwayStateKey(session, cid)
	if client != nil && client.membership != nil {
		key = runwayStateKey(session, client.membership.MembershipId)
	}
	state, ok := hub.runwayStates[key]
	if !ok {
		state = &clientRunwayState{
			Session: session,
			CID:     cid,
			Client:  client,
		}
		hub.runwayStates[key] = state
	}

	prevDepartureMismatch := state.DepartureMismatch
	prevArrivalMismatch := state.ArrivalMismatch

	state.Callsign = callsign
	state.DepartureRunways = current.DepartureRunways
	state.ArrivalRunways = current.ArrivalRunways

	if isMaster {
		state.DepartureMismatch = false
		state.ArrivalMismatch = false
		state.LastAlertSignature = ""
	} else {
		state.DepartureMismatch = !slicesEqual(master.DepartureRunways, current.DepartureRunways)
		state.ArrivalMismatch = !slicesEqual(master.ArrivalRunways, current.ArrivalRunways)

		if !state.DepartureMismatch && !state.ArrivalMismatch {
			state.LastAlertSignature = ""
		} else {
			signature := hub.GetMasterCallsign(session) + "|" + runwayMismatchSignature(master, current)
			if state.LastAlertSignature != signature {
				alert := buildRunwayMismatchAlert(master, current)
				alert.MasterCallsign = hub.GetMasterCallsign(session)
				evaluation.Alert = &alert
				state.LastAlertSignature = signature
			}
		}
	}

	evaluation.DepartureMismatch = state.DepartureMismatch
	evaluation.ArrivalMismatch = state.ArrivalMismatch
	evaluation.Changed = prevDepartureMismatch != state.DepartureMismatch || prevArrivalMismatch != state.ArrivalMismatch

	return evaluation
}

func (hub *Hub) resyncSessionRunwayMismatchTargets(session int32, masterCID string, master models.ActiveRunways) {
	master = normalizeActiveRunways(master)
	currentMaster := hub.getMasterClient(session)

	hub.runwayStateMu.Lock()
	evaluations := make([]runwayClientEvaluation, 0)
	for _, state := range hub.runwayStates {
		if state.Session != session {
			continue
		}

		current := models.ActiveRunways{
			DepartureRunways: state.DepartureRunways,
			ArrivalRunways:   state.ArrivalRunways,
		}
		isMaster := state.CID == masterCID
		if state.Client != nil {
			isMaster = state.Client == currentMaster
		}

		if isMaster {
			state.DepartureMismatch = false
			state.ArrivalMismatch = false
			state.LastAlertSignature = ""
			continue
		}

		state.DepartureMismatch = !slicesEqual(master.DepartureRunways, current.DepartureRunways)
		state.ArrivalMismatch = !slicesEqual(master.ArrivalRunways, current.ArrivalRunways)

		if !state.DepartureMismatch && !state.ArrivalMismatch {
			state.LastAlertSignature = ""
			continue
		}

		evaluation := runwayClientEvaluation{
			CID:               state.CID,
			Target:            state.Client,
			DepartureMismatch: state.DepartureMismatch,
			ArrivalMismatch:   state.ArrivalMismatch,
		}

		signature := hub.GetMasterCallsign(session) + "|" + runwayMismatchSignature(master, current)
		if state.LastAlertSignature != signature {
			alert := buildRunwayMismatchAlert(master, current)
			alert.MasterCallsign = hub.GetMasterCallsign(session)
			evaluation.Alert = &alert
			state.LastAlertSignature = signature
		}

		evaluations = append(evaluations, evaluation)
	}
	hub.runwayStateMu.Unlock()

	if hub.server == nil {
		return
	}

	frontendHub := hub.server.GetFrontendHub()
	for _, evaluation := range evaluations {
		if frontendHub != nil {
			sendFrontendRunwayConfiguration(frontendHub, evaluation.Target, session, evaluation.CID, frontendEvents.RunwayConfigurationEvent{
				RunwaySetup: buildFrontendRunwayConfiguration(master, evaluation.DepartureMismatch, evaluation.ArrivalMismatch),
			})
		}
		if evaluation.Alert != nil {
			if evaluation.Target != nil {
				evaluation.Target.Enqueue(*evaluation.Alert)
			} else {
				hub.Send(session, evaluation.CID, *evaluation.Alert)
			}
		}
	}
}

func (hub *Hub) clearMembershipRunwayState(client *Client) {
	if client.membership == nil {
		hub.clearClientRunwayState(client.session, client.GetCid())
		return
	}
	hub.runwayStateMu.Lock()
	delete(hub.runwayStates, runwayStateKey(client.session, client.membership.MembershipId))
	hub.runwayStateMu.Unlock()
}
