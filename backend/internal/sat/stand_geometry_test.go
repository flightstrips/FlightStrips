package sat

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStandAtPosition(t *testing.T) {
	registry, err := LoadStandCapabilities(strings.NewReader(`
STAND:EKCH:A1:N055.37.42.710:E012.38.33.450:30
STAND:EKCH:A2:N055.37.42.710:E012.38.36.450:30
`))
	require.NoError(t, err)

	stand, found := registry.StandAtPosition("ekch", 55.6285306, 12.642625)
	require.True(t, found)
	assert.Equal(t, "A1", stand.Name)

	_, found = registry.StandAtPosition("EKCH", 55.7, 12.7)
	assert.False(t, found)
}
