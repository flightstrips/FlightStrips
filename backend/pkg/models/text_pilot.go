package models

type TextPilotInputs struct {
	Airport        string                       `json:"airport"`
	PresetProvider string                       `json:"preset_provider"`
	Layouts        map[string]TextPilotLayout   `json:"layouts"`
	Presets        map[string][]TextPilotPhrase `json:"presets,omitempty"`
}

type TextPilotBox struct {
	Label      string  `json:"label,omitempty"`
	Left       float64 `json:"left"`
	Top        float64 `json:"top"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	FontSize   float64 `json:"fontSize,omitempty"`
	Background string  `json:"background,omitempty"`
	Color      string  `json:"color,omitempty"`
	FontWeight float64 `json:"fontWeight,omitempty"`
	PhraseID   string  `json:"phraseId,omitempty"`
}

type TextPilotLayout struct {
	Width    float64        `json:"width"`
	Height   float64        `json:"height"`
	Frame    TextPilotBox   `json:"frame"`
	Boxes    []TextPilotBox `json:"boxes"`
	Dividers []TextPilotBox `json:"dividers,omitempty"`
}

type TextPilotPhrase struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Text  string `json:"text"`
	Tone  string `json:"tone"`
}
