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

func TestConfigDefaultsBundledTerminalGeometry(t *testing.T) {
	config := Config{Source: SourceAIRACNet}.Normalize()
	require.Equal(t, DefaultTerminalGeometryPath, config.TerminalGeometryPath)
	require.NoError(t, config.Validate())
}

func TestConfigRejectsTerminalGeometryWithoutSource(t *testing.T) {
	require.ErrorContains(t, (Config{TerminalGeometryPath: "terminal.json"}).Validate(), "requires a navigation source")
}
