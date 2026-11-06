package frontend

// BoardDefinition is immutable startup metadata for a compiled frontend view.
// Actions come from the bound workflow, service flags from the selected definition.
// Readiness and observer state belong to each own membership, not this catalog.
type BoardDefinition struct {
	DefinitionRevision string            `json:"definition_revision"`
	View               string            `json:"view"`
	Actions            []string          `json:"actions"`
	Capabilities       BoardCapabilities `json:"capabilities"`
}

type BoardCapabilities struct {
	CDM       bool `json:"cdm"`
	Stands    bool `json:"stands"`
	TextPilot bool `json:"text_pilot"`
	PDC       bool `json:"pdc"`
	AMAN      bool `json:"aman"`
}
