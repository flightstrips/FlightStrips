package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNATSConfigRequiresExplicitSharedKeyRing(t *testing.T) {
	t.Setenv("NATS_URLS", "nats://localhost:4222")
	t.Setenv("NATS_EFFECT_ACTIVE_KEY_ID", "v2")
	t.Setenv("NATS_EFFECT_KEY_FILES", `v1=C:\secrets\v1.key,v2=C:\secrets\v2.key`)
	t.Setenv("NATS_AIRPORTS", "ekch,EKBI")
	cfg, err := natsConfigFromEnv()
	require.NoError(t, err)
	require.Equal(t, `C:\secrets\v2.key`, cfg.EffectKeyFiles["v2"])
	require.Equal(t, []string{"EKCH", "EKBI"}, cfg.Airports)
	for _, ring := range []string{"", "v1=a", "v2=", "v2=a,v2=b", "v2=a,b", "=a"} {
		t.Setenv("NATS_EFFECT_KEY_FILES", ring)
		_, err = natsConfigFromEnv()
		require.Error(t, err, ring)
	}
	t.Setenv("NATS_EFFECT_KEY_FILES", "v2=a")
	t.Setenv("NATS_AIRPORTS", "EKCH,invalid")
	_, err = natsConfigFromEnv()
	require.Error(t, err)
}
