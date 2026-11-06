package textpilot

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestInputsRequireOwnAirportAndExplicitPresetProvider(t *testing.T) {
	for _, invalid := range []string{`{"airport":"AAAA","preset_provider":"ekch/v1"}`, `{"airport":"BBBB","preset_provider":"static/v1"}`, `{"airport":"AAAA","preset_provider":"static/v1"}`, `{"airport":"AAAA","preset_provider":"unknown"}`} {
		_, err := LoadInputs("AAAA", []byte(invalid))
		require.Error(t, err)
	}
	input, err := LoadInputs("AAAA", []byte(`{"airport":"AAAA","preset_provider":"static/v1","layouts":{},"presets":{"si":[{"id":"hold","label":"LOCAL HOLD","text":"Synthetic hold.","tone":"phrase"}]}}`))
	require.NoError(t, err)
	require.Equal(t, "Synthetic hold.", input.Presets["si"][0].Text)
}
