package frontend

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"FlightStrips/internal/aman"
	events "FlightStrips/pkg/events/frontend"
)

// Controllers without FMP authority may change the order of flights on the
// STAR families their ACC view gives them. This mirrors defaultACCFamilies in
// frontend/src/lib/aman-default-families.ts; keep the two in step.

var (
	swedishSTARFamilies = map[string]bool{"TIDVU": true, "ERNOV": true}
	allEKDKCentre       = map[string]bool{"EKDK_UC_CTR": true, "EKDK_UV_CTR": true, "EKDK_UN_CTR": true, "EKDK_CTR": true}
	allEKCHApproach     = map[string]bool{"EKCH_P_APP": true, "EKCH_P1_APP": true, "EKCH_P2_APP": true}
	tidvuOwners         = []string{"ESMS_APP", "ESMM_7_CTR", "ESMM_8_CTR", "ESMM_Y_CTR"}
	ernovOwners         = []string{"ESMM_5_CTR", "ESMM_K_CTR"}
)

// controllerSequencingFamilies returns the STAR families a controller may
// sequence. online holds upper-case callsigns of logged-on non-observer
// controllers; available holds the families on the AMAN timelines.
func controllerSequencingFamilies(position string, online map[string]bool, available map[string]bool) map[string]bool {
	own := strings.ToUpper(strings.TrimSpace(position))
	isOnline := func(callsign string) bool { return callsign == own || online[callsign] }
	offline := func(callsign string) bool { return !isOnline(callsign) }
	anyOnline := func(callsigns []string) bool { return slices.ContainsFunc(callsigns, isOnline) }
	selected := map[string]bool{}
	add := func(families ...string) {
		for _, family := range families {
			selected[family] = true
		}
	}

	switch own {
	case "EKDK_B_CTR":
		add("MONAK")
		if offline("EKDK_D_CTR") || offline("EKDK_E_CTR") || offline("EKDK_I_CTR") {
			add("TUDLO", "TESPI")
		}
	case "EKDK_I_CTR":
		if offline("EKDK_B_CTR") {
			add("MONAK")
		}
		if offline("EKDK_D_CTR") {
			add("TUDLO")
		}
		if offline("EKDK_E_CTR") {
			add("TESPI")
		}
	case "EKDK_D_CTR":
		add("TUDLO")
		if offline("EKDK_E_CTR") {
			add("TESPI")
		}
		if offline("EKDK_B_CTR") || offline("EKDK_I_CTR") {
			add("MONAK")
		}
	case "EKDK_E_CTR":
		add("TESPI")
		if offline("EKDK_D_CTR") {
			add("TUDLO")
		}
		if offline("EKDK_I_CTR") && offline("EKDK_B_CTR") && offline("EKDK_D_CTR") {
			add("MONAK")
		}
	case "ESMS_APP", "ESMM_7_CTR", "ESMM_8_CTR", "ESMM_Y_CTR":
		add("TIDVU")
	case "ESMM_5_CTR":
		add("ERNOV")
	case "ESMM_K_CTR":
		add("ERNOV")
		if !anyOnline(tidvuOwners) {
			add("TIDVU")
		}
	case "ESMM_2_CTR":
		if !anyOnline(tidvuOwners) {
			add("TIDVU")
		}
		if !anyOnline(ernovOwners) {
			add("ERNOV")
		}
	case "EKCH_W_APP":
		add("TESPI", "TUDLO", "ERNOV")
	case "EKCH_O_APP":
		add("TIDVU", "MONAK")
	default:
		switch {
		case allEKDKCentre[own]:
			for family := range available {
				if !swedishSTARFamilies[family] {
					add(family)
				}
			}
		case allEKCHApproach[own]:
			// The ACC view shows every family to the combined approach.
			for family := range available {
				add(family)
			}
		}
	}
	for family := range selected {
		if !available[family] {
			delete(selected, family)
		}
	}
	return selected
}

func availableSTARFamilies(state events.AMANState) map[string]bool {
	families := map[string]bool{}
	if state.TimelineConfig == nil {
		return families
	}
	for _, mapping := range state.TimelineConfig.Mappings {
		for _, side := range []*string{mapping.Left, mapping.Right} {
			if side != nil && *side != "" {
				families[strings.ToUpper(*side)] = true
			}
		}
	}
	return families
}

// onlineControllerCallsigns lists logged-on non-observer controllers. A
// failed lookup is an error, never "everyone is offline", because offline
// neighbours widen a controller's STAR families.
func (hub *Hub) onlineControllerCallsigns(ctx context.Context, session int32) (map[string]bool, error) {
	online := map[string]bool{}
	if hub.server == nil || hub.server.GetControllerRepository() == nil {
		return nil, errControllerPresenceUnavailable
	}
	controllers, err := hub.server.GetControllerRepository().ListBySession(ctx, session)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errControllerPresenceUnavailable, err)
	}
	for _, controller := range controllers {
		if controller != nil && !controller.Observer {
			online[strings.ToUpper(strings.TrimSpace(controller.Callsign))] = true
		}
	}
	return online, nil
}

var errNoSequencingState = errors.New("AMAN state is unavailable for controller sequencing")

// authorizeControllerSequencing checks that every flight a sequence-order
// command names, and any STAR family it assigns, belongs to the controller's
// own STAR families.
func (hub *Hub) authorizeControllerSequencing(ctx context.Context, identity clientIdentity, callsigns []aman.Callsign, starFamily *string) error {
	if hub.amanStateProvider == nil {
		return errNoSequencingState
	}
	event, err := hub.amanStateProvider.CurrentAMANState(aman.WithSession(ctx, identity.session), identity.airport)
	if err != nil {
		return fmt.Errorf("%w: %v", errNoSequencingState, err)
	}
	position := hub.amanRoleForIdentity(identity)
	online, err := hub.onlineControllerCallsigns(ctx, identity.session)
	if err != nil {
		return err
	}
	families := controllerSequencingFamilies(position, online, availableSTARFamilies(event.Data))
	if len(families) == 0 {
		return &aman.DomainError{Class: aman.ErrorUnauthorized, Message: fmt.Sprintf("%s has no STAR families to sequence; FMP authority is required", position)}
	}
	own := make([]string, 0, len(families))
	for family := range families {
		own = append(own, family)
	}
	slices.Sort(own)
	outside := func(what string) error {
		return &aman.DomainError{Class: aman.ErrorUnauthorized, Message: fmt.Sprintf("%s is not on your STAR families (%s)", what, strings.Join(own, ", "))}
	}
	if starFamily != nil && !families[strings.ToUpper(strings.TrimSpace(*starFamily))] {
		return outside("STAR family " + *starFamily)
	}
	byCallsign := make(map[string]events.AMANFlight, len(event.Data.Flights))
	for _, flight := range event.Data.Flights {
		byCallsign[strings.ToUpper(flight.Callsign)] = flight
	}
	for _, callsign := range callsigns {
		flight, ok := byCallsign[strings.ToUpper(string(callsign))]
		if !ok || flight.STARFamily == nil || !families[strings.ToUpper(*flight.STARFamily)] {
			return outside(string(callsign))
		}
	}
	return nil
}

// amanSequenceContext authorizes a sequence-order command. FMP authority
// allows any flight; other controllers are limited to their STAR families.
func (hub *Hub) amanSequenceContext(ctx context.Context, client *Client, callsigns []aman.Callsign, starFamily *string) (aman.CommandContext, error) {
	identity := client.messageIdentity(ctx)
	if hub.hasAMANFMPAuthorityForIdentity(client, identity) {
		return hub.amanContextForIdentity(client, identity)
	}
	auth, err := hub.coordinationContextForIdentity(client, identity)
	if err != nil {
		return aman.CommandContext{}, err
	}
	if identity.session == WaitingForEuroscopeConnectionSessionId {
		return aman.CommandContext{}, &aman.DomainError{Class: aman.ErrorUnauthorized, Message: "AMAN sequencing requires a EuroScope session"}
	}
	if err := hub.authorizeControllerSequencing(ctx, identity, callsigns, starFamily); err != nil {
		var domain *aman.DomainError
		if errors.As(err, &domain) {
			return aman.CommandContext{}, err
		}
		return aman.CommandContext{}, &aman.DomainError{Class: aman.ErrorDependencyUnavailable, Message: "AMAN state is unavailable; try again"}
	}
	return aman.CommandContext{SessionID: identity.session, Airport: auth.Airport, Actor: auth.Actor, Role: auth.Role, ReceivedAt: auth.ReceivedAt, ControllerSequencing: true}, nil
}

func runAMANSequenceCommand(ctx context.Context, client *Client, commandID string, callsigns []aman.Callsign, starFamily *string, execute func(aman.CommandContext) (aman.CommandExecution, error)) error {
	auth, err := client.hub.amanSequenceContext(ctx, client, callsigns, starFamily)
	if err != nil {
		return rejectDecodedAMAN(ctx, client, commandID, err)
	}
	return executeAMANCommand(ctx, client, commandID, auth, execute)
}
