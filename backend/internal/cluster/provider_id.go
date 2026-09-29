package cluster

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ProviderEventCommandID derives one command UUID from a provider's stable
// event identity. The family separates unrelated adapters sharing an event ID.
func ProviderEventCommandID(family, provider, eventID string) (string, error) {
	family = strings.ToLower(strings.TrimSpace(family))
	provider = strings.ToLower(strings.TrimSpace(provider))
	eventID = strings.TrimSpace(eventID)
	if family == "" || provider == "" || eventID == "" || strings.ContainsRune(family, 0) || strings.ContainsRune(provider, 0) || strings.ContainsRune(eventID, 0) {
		return "", fmt.Errorf("missing or invalid provider event identity")
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte("flightstrips/"+family+"\x00"+provider+"\x00"+eventID)).String(), nil
}
