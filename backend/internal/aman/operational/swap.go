package operational

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/sequence"
)

func (s *Service) SwapFlights(auth aman.CommandContext, command aman.SwapFlightsCommand) (sequence.CommandMutation, error) {
	return func(state aman.AirportState) (sequence.CommandChange, error) {
		state.Flights = append([]aman.AMANFlight(nil), state.Flights...)
		invalidateHoldingRebuild(&state, command.Callsign)
		invalidateHoldingRebuild(&state, command.OtherCallsign)
		decision, err := sequence.ApplySwap(s.sequenceInput(state), command)
		if err != nil {
			return sequence.CommandChange{}, err
		}
		return s.commandChange(s.applyDecision(state, decision), decision.Changed, "swap_flights", command.Callsign, map[string]any{
			"other_callsign": command.OtherCallsign, "actor": auth.Actor, "role": auth.Role,
		})
	}, nil
}
