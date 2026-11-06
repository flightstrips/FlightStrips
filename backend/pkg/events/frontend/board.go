package frontend

import (
	"FlightStrips/pkg/events"
	"encoding/json"
)

// BoundMessage attaches the socket's immutable selection to every board event,
// including rejection and deferred results. Business payloads remain unchanged.
type BoundMessage struct {
	Message events.OutgoingMessage
	Board   events.BoardContext
}

func (b BoundMessage) Marshal() ([]byte, error) {
	data, err := b.Message.Marshal()
	if err != nil {
		return nil, err
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		return nil, err
	}
	scope, err := json.Marshal(b.Board)
	if err != nil {
		return nil, err
	}
	object["board"] = scope
	return json.Marshal(object)
}

// BoardDefinition is immutable startup metadata for a compiled frontend view.
// Actions come from the bound workflow, service flags from the selected definition.
// Readiness and observer state belong to each own membership, not this catalog.
type BoardDefinition struct {
	Runways            []string            `json:"runways,omitempty"`
	BoardState         *StripBoardSettings `json:"board_state,omitempty"`
	DefinitionRevision string              `json:"definition_revision"`
	View               string              `json:"view"`
	Actions            []string            `json:"actions"`
	Capabilities       BoardCapabilities   `json:"capabilities"`
}

type BoardCapabilities struct {
	CDM       bool `json:"cdm"`
	Stands    bool `json:"stands"`
	TextPilot bool `json:"text_pilot"`
	PDC       bool `json:"pdc"`
	AMAN      bool `json:"aman"`
}

type StripBoardSettings struct {
	Revisioned      bool `json:"revisioned"`
	RecoverySeconds int  `json:"recovery_seconds"`
}
