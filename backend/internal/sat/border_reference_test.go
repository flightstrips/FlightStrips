package sat

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAirportCountryRegistryClassifiesExactAndPrefixCodes(t *testing.T) {
	registry := NewAirportCountryRegistry()

	assert.Equal(t, BorderStatusSchengen, registry.BorderStatus("EKCH"))
	assert.Equal(t, BorderStatusSchengen, registry.BorderStatus("ELLX"))
	assert.Equal(t, BorderStatusNonSchengen, registry.BorderStatus("kjfk"))
	assert.Equal(t, BorderStatusNonSchengen, registry.BorderStatus("ZBAA"))
	assert.Equal(t, BorderStatusUnknown, registry.BorderStatus("XXXX"))
	assert.Equal(t, BorderStatusUnknown, registry.BorderStatus("ZZZZ"))
}
