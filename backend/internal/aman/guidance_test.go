package aman

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGainLossGuidance(t *testing.T) {
	target := time.Date(2026, 10, 4, 8, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		drift time.Duration
		want  int64
	}{{90 * time.Second, 90}, {2 * time.Minute, 120}, {45 * time.Minute, 120}, {-100 * time.Minute, -6000}, {29900 * time.Millisecond, 30}} {
		seconds, err := GainLossGuidance(target.Add(test.drift), target)
		require.NoError(t, err)
		require.Equal(t, test.want, seconds)
	}
}
