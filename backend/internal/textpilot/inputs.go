package textpilot

import (
	"FlightStrips/pkg/models"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func LoadInputs(airport string, data []byte) (*models.TextPilotInputs, error) {
	var input models.TextPilotInputs
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		return nil, err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("text pilot inputs must contain one JSON object")
	}
	if input.Airport != airport {
		return nil, fmt.Errorf("text pilot inputs must describe airport %s", airport)
	}
	switch input.PresetProvider {
	case "ekch/v1":
		if airport != "EKCH" {
			return nil, fmt.Errorf("EKCH text pilot presets cannot be used for %s", airport)
		}
	case "static/v1":
		if len(input.Presets) == 0 {
			return nil, fmt.Errorf("static text pilot presets are required")
		}
	default:
		return nil, fmt.Errorf("unknown text pilot preset provider %q", input.PresetProvider)
	}
	for variant, layout := range input.Layouts {
		if !validVariant(variant) || layout.Width <= 0 || layout.Height <= 0 {
			return nil, fmt.Errorf("invalid text pilot layout %q", variant)
		}
	}
	for variant, phrases := range input.Presets {
		if !validVariant(variant) {
			return nil, fmt.Errorf("invalid text pilot variant %q", variant)
		}
		seen := make(map[string]bool)
		for _, phrase := range phrases {
			if phrase.ID == "" || seen[phrase.ID] || phrase.Label == "" || strings.ContainsAny(phrase.Text, "\r\n\x00") || (phrase.Tone != "light" && phrase.Tone != "phrase" && phrase.Tone != "point") {
				return nil, fmt.Errorf("invalid text pilot phrase in %s", variant)
			}
			seen[phrase.ID] = true
		}
	}
	return &input, nil
}

func validVariant(variant string) bool {
	switch variant {
	case "push", "apron-out", "apron-in", "tower-out", "tower-in", "si", "runway", "clearance":
		return true
	}
	return false
}
