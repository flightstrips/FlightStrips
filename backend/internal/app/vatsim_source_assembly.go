package app

import (
	"FlightStrips/internal/vatsim"
	"log/slog"
)

type vatsimSourceAssembly struct {
	cache     *vatsim.Cache
	synthetic *vatsim.SyntheticSource
	source    vatsim.FlightSource
}

func assembleVATSIMSource(cfg Config, deps Dependencies, requireLiveCIDVerification bool, enableReconciliation bool) vatsimSourceAssembly {
	if cfg.EnableTestTools {
		source := vatsim.NewSyntheticSource()
		slog.Warn("Local test tools enabled; external VATSIM feed disabled")
		return vatsimSourceAssembly{synthetic: source, source: source}
	}

	cache := buildVATSIMCache(cfg, deps, requireLiveCIDVerification, enableReconciliation)
	assembly := vatsimSourceAssembly{cache: cache}
	if cache != nil {
		assembly.source = cache
	}
	return assembly
}
