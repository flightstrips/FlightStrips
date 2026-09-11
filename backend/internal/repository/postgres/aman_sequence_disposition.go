package postgres

import (
	"FlightStrips/internal/aman"
	"encoding/json"
	"fmt"
)

// decodeSequenceDisposition owns the additive persistence contract for the
// field. Absence means the payload predates disposition support; a present
// value must be one of the domain vocabulary values, including when it is null
// or an empty string.
func decodeSequenceDisposition(encoded []byte) (aman.SequenceDisposition, error) {
	var compatibility struct {
		SequenceDisposition json.RawMessage
	}
	if err := json.Unmarshal(encoded, &compatibility); err != nil {
		return "", err
	}
	if len(compatibility.SequenceDisposition) == 0 {
		return aman.SequenceDispositionActive, nil
	}

	var disposition aman.SequenceDisposition
	if err := json.Unmarshal(compatibility.SequenceDisposition, &disposition); err != nil {
		return "", fmt.Errorf("decode sequence disposition: %w", err)
	}
	if !disposition.Valid() {
		return "", fmt.Errorf("invalid sequence disposition %q", disposition)
	}
	return disposition, nil
}
