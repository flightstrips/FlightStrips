package euroscope

import (
	"errors"
	"slices"
	"strings"
	"time"

	"FlightStrips/internal/config"
	"FlightStrips/internal/vatsim"
)

type coverageTakeover struct {
	Deadline     time.Time
	Replacements []string
}

// ReconcileCoverage applies only changed airport memberships through existing
// join/remove, synchronization, election and offline paths. The caller supplies
// server authority and one environment's shared normalized evidence. No source
// acquisition, operational heartbeat or strip ownership is involved here.
func (hub *Hub) ReconcileCoverage(root *Client, declared []string, authorized func(string) bool, staffing CoverageStaffingEvidence, now time.Time) error {
	if root == nil || root.transport == nil {
		return nil
	}
	t := root.transport
	t.coverageMu.Lock()
	defer t.coverageMu.Unlock()
	return hub.reconcileCoverageLocked(root, declared, authorized, staffing, now)
}

func (hub *Hub) reconcileCoverageLocked(root *Client, declared []string, authorized func(string) bool, staffing CoverageStaffingEvidence, now time.Time) error {
	if root.isClosed() || !root.IsAuthenticated() || root.IsDraining() || hub.airports == nil {
		return nil
	}
	t := root.transport
	t.mu.RLock()
	control := t.evidence
	t.mu.RUnlock()
	if !environmentName.MatchString(control.Environment) {
		return nil
	}
	if t.takeovers == nil {
		t.takeovers = map[string]coverageTakeover{}
	}
	allowed := map[string]bool{}
	for _, definition := range hub.airports.CoverageAirports(declared, authorized) {
		allowed[definition.Airport()] = true
	}
	members := map[string]*Client{}
	for _, member := range t.snapshot() {
		members[member.airport] = member
	}
	key := coverageControllerKey(root.GetCid(), control.Callsign)
	if staffing.ESOnly || simulatedPrimaryOnly(control.Environment) {
		key = coverageControllerKey("", control.Callsign)
	}
	var self *CoverageControllerEvidence
	if own, exists := staffing.instances[coverageInstanceKey{root.GetCid(), t.instance, t.generation}]; exists {
		self = &own
	}
	participants := make([]config.CoverageParticipant, 0, len(staffing.Controllers))
	for i := range staffing.Controllers {
		entry := &staffing.Controllers[i]
		participant := coverageParticipantForStaffing(entry.CoverageParticipant, staffing)
		if participant.Key == key && self != nil {
			entry = self
			participant = coverageParticipantForStaffing(entry.CoverageParticipant, staffing)
		}
		participants = append(participants, participant)
		if participant.Key == key {
			self = entry
		}
	}
	var result error
	for _, airport := range hub.airports.Airports() {
		definition, _ := hub.airports.Lookup(airport)
		if definition.MembershipPolicy() != "remaining-responsibilities/v1" {
			continue
		}
		member := members[airport]
		remove := func(reason string) { hub.RemoveMembership(member, reason); delete(t.takeovers, airport) }
		if !allowed[airport] || control.Observer || control.Callsign == "" {
			remove("airport participation is no longer authorized")
			continue
		}
		complete := staffing.Environment == control.Environment && coverageEvidenceComplete(definition, staffing, key, self) && self != nil && self.PrimaryFrequency == vatsim.NormalizeFrequency(control.PrimaryFrequency)
		pending, confirmed := t.takeovers[airport]
		if complete {
			assignments := definition.CoverageResponsibilities(participants)
			if len(assignments[key]) > 0 {
				delete(t.takeovers, airport)
				if member == nil {
					_, err := hub.JoinMembership(root, airport)
					result = errors.Join(result, err)
				}
				continue
			}
			potential := definition.CoverageResponsibilities([]config.CoverageParticipant{coverageParticipantForStaffing(self.CoverageParticipant, staffing)})[key]
			if len(potential) == 0 {
				remove("instance no longer covers airport responsibilities")
				continue
			}
			if member == nil {
				delete(t.takeovers, airport)
				continue
			}
			replacements := []string{}
			for candidate, duties := range assignments {
				if slices.ContainsFunc(duties, func(duty string) bool { return slices.Contains(potential, duty) }) {
					replacements = append(replacements, candidate)
				}
			}
			slices.Sort(replacements)
			if !confirmed {
				pending.Deadline = now.Add(definition.RelinquishGrace())
			}
			pending.Replacements = replacements
			t.takeovers[airport] = pending
			confirmed = true
		}
		// Once confirmed, transient unknown/stale evidence cannot extend the
		// agreed deadline. Fresh evidence returning a duty cancels it above.
		if confirmed && (!now.Before(pending.Deadline) || hub.coverageReplacementsAttached(airport, control.Environment, pending.Replacements)) {
			remove("airport responsibilities taken over")
		}
	}
	return result
}

func coverageEvidenceComplete(definition *config.AirportDefinition, staffing CoverageStaffingEvidence, key string, self *CoverageControllerEvidence) bool {
	if staffing.Inventory != CoverageFresh {
		return false
	}
	needsRadios := definition.CoverageUsesTunedFrequencies() && !staffing.ESOnly && !simulatedPrimaryOnly(staffing.Environment)
	for _, controller := range staffing.Controllers {
		if coverageParticipantForStaffing(controller.CoverageParticipant, staffing).Key == key && self != nil {
			controller = *self
		}
		if controller.Observer {
			continue
		}
		if controller.Identity != CoverageFresh || (needsRadios && controller.Radios != CoverageFresh) {
			return false
		}
	}
	return true
}

func simulatedPrimaryOnly(environment string) bool {
	return environment == "SWEATBOX" || strings.HasPrefix(environment, "SWEATBOX_") || strings.HasPrefix(environment, "PLAYBACK_")
}

// ES visible-roster participation uses configured primary/callsign evidence.
// Transceiver-driven sector selection remains a separate server operation.
// Legacy in-process providers retain their configured radio policy for LIVE.
// Use the same policy for assignments and takeover potential.
func coverageParticipantForStaffing(participant config.CoverageParticipant, staffing CoverageStaffingEvidence) config.CoverageParticipant {
	if staffing.ESOnly || simulatedPrimaryOnly(staffing.Environment) {
		// Every instance must use the same tie-break key for a visible callsign,
		// regardless of which peer is its authenticated local controller.
		participant.Key = coverageControllerKey("", participant.Callsign)
		participant.TunedFrequencies = nil
	}
	return participant
}

func (hub *Hub) coverageReplacementsAttached(airport, environment string, keys []string) bool {
	if len(keys) == 0 {
		return false
	}
	attached := map[string]bool{}
	for _, member := range hub.clientsSnapshot() {
		if member.membership == nil || member.isClosed() || !member.IsAuthenticated() || member.airport != airport || member.sessionName != environment || member.identitySnapshot().observer {
			continue
		}
		attached[coverageControllerKey(member.GetCid(), member.GetCallsign())] = true
		// SDK-visible peers have callsigns, not authenticated CIDs. Match
		// their attachment in this same airport/environment by callsign.
		attached[coverageControllerKey("", member.GetCallsign())] = true
	}
	for _, key := range keys {
		if !attached[key] {
			return false
		}
	}
	return true
}
