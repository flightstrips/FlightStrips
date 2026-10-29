package main

import (
	"context"
	"path/filepath"
	"testing"

	"FlightStrips/internal/aman/navdata/airacnet"
	"FlightStrips/internal/aman/terminal"

	"github.com/stretchr/testify/require"
)

func TestAMANCycleMatchesTerminalConfiguration(t *testing.T) {
	suite := newAMANSuite()
	defer suite.close()
	config, err := terminal.LoadFile(filepath.Join("..", "..", "..", suite.config.Navigation.TerminalGeometryPath))
	require.NoError(t, err)
	adapter, err := airacnet.New(airacnet.Config{Checkpoints: airacnet.NewMemoryCheckpoints(), Now: suite.dependencies.Now})
	require.NoError(t, err)
	version, err := adapter.LatestVersion(context.Background())
	require.NoError(t, err)
	require.Equal(t, config.Dataset.Cycle, version.Cycle)
	require.Equal(t, config.Dataset.EffectiveFrom, version.EffectiveFrom)
	require.Equal(t, config.Dataset.EffectiveUntil, version.EffectiveUntil)
	now := suite.dependencies.Now()
	require.False(t, now.Before(config.ApplicabilityFrom))
	require.True(t, now.Before(config.ApplicabilityUntil))
}
