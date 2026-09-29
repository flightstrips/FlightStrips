package pdc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"FlightStrips/internal/cluster"
)

// HoppieMessageCommandID is stable across polling retries. Hoppie messages do
// not expose a separate event ID, so the complete provider frame is its ID.
func HoppieMessageCommandID(message Message) (string, error) {
	raw := strings.TrimSpace(message.Raw)
	if raw == "" {
		return "", fmt.Errorf("missing Hoppie provider frame")
	}
	digest := sha256.Sum256([]byte(raw))
	return cluster.ProviderEventCommandID("pdc-response", "hoppie", hex.EncodeToString(digest[:]))
}
