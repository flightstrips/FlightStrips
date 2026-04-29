package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetTaxiwayTypeValidationScopeName_UsesTowerScopeForTwrGnd(t *testing.T) {
	original := layouts
	layouts = map[string][]LayoutVariant{
		"EKCH_A_TWR": {
			{Layout: "TWRGND"},
		},
	}
	t.Cleanup(func() { layouts = original })

	scopeName, ok := getTaxiwayTypeValidationScopeName("EKCH_A_TWR")
	require.True(t, ok)
	assert.Equal(t, taxiwayTypeValidationScopeTower, scopeName)
}
