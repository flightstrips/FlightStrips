package navigation

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestConfigAllowsNavigationToRemainDisabled(t *testing.T) {
	require.NoError(t, (Config{}).Validate())
}

func TestConfigNormalizesAndValidatesAIRACNet(t *testing.T) {
	config := Config{Source: " AIRACNET ", TerminalGeometryPath: " terminal.json "}.Normalize()
	require.Equal(t, Config{Source: SourceAIRACNet, TerminalGeometryPath: "terminal.json"}, config)
	require.NoError(t, config.Validate())
}

func TestConfigRequiresSourceAndTerminalGeometryTogether(t *testing.T) {
	require.ErrorContains(t, (Config{TerminalGeometryPath: "terminal.json"}).Validate(), "requires a navigation source")
	require.ErrorContains(t, (Config{Source: SourceAIRACNet}).Validate(), "terminal geometry path")
}
