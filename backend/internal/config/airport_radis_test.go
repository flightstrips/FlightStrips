package config

import (
	"bytes"
	"github.com/stretchr/testify/require"
	"os"
	"testing"
)

func TestRADISTypedConfigurationRejectsUnreviewedOrIncompatibleInputs(t *testing.T) {
	data, err := os.ReadFile("testdata/radis-ekbi.yaml")
	require.NoError(t, err)
	d, err := DecodeAirport("synthetic", bytes.NewReader(data))
	require.NoError(t, err)
	require.IsType(t, RADISSettings{}, d.Workflow.Settings)
	for _, change := range []struct{ from, to string }{{"strips: none", "strips: route"}, {"recovery_seconds: 123", "recovery_seconds: 0"}, {"timestamp_semantics: annotation-utc", "timestamp_semantics: rea-eobt"}, {"departure_removal_agl_ft: 3100", "departure_removal_agl_ft: 100"}, {"type: timestamp", "type: string"}, {"admission_distance_nm: 37", "admission_distance_nm: 37\n    script: arbitrary"}} {
		mutated := bytes.Replace(data, []byte(change.from), []byte(change.to), 1)
		require.NotEqual(t, data, mutated)
		_, err := DecodeAirport("bad synthetic", bytes.NewReader(mutated))
		require.Error(t, err, change.from)
	}
}
