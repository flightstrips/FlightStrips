package frontendbinary

import (
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
)

type deliveryProjection struct {
	*fixtureProjection
	awaited bool
	failure error
}

func (p *deliveryProjection) Ready() error        { return errors.New("ordinary replay gap") }
func (p *deliveryProjection) ReadyForRead() error { p.awaited = true; return p.failure }

func TestFrontendUsesReplayBarrierAndPropagatesHealthFailure(t *testing.T) {
	p := &deliveryProjection{fixtureProjection: frontendFixture()}
	require.NoError(t, readyForDelivery(p))
	require.True(t, p.awaited)
	p.failure = errors.New("quorum unavailable")
	require.ErrorIs(t, readyForDelivery(p), p.failure)
}
