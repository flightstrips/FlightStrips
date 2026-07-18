package vatsim

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLegacyArrivalETAWriterDefaultsOnAndCanBeDisabled(t *testing.T) {
	base := newReconciler(nil, nil, nil, nil, nil, nil, nil, time.Second)
	require.True(t, base.legacyArrivalETAWriter)

	disabled := newReconciler(nil, nil, nil, nil, nil, nil, nil, time.Second, WithLegacyArrivalETAWriter(false))
	require.False(t, disabled.legacyArrivalETAWriter)
}
