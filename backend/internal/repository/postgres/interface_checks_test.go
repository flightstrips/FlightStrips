package postgres_test

import (
	"FlightStrips/internal/cdm"
	"FlightStrips/internal/ecfmp"
	"FlightStrips/internal/frontend"
	"FlightStrips/internal/pdc"
	"FlightStrips/internal/repository"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/services"
)

var (
	_ repository.StandAssignmentRepository = postgres.NewStandAssignmentRepository(nil)
	_ pdc.PdcStripStore                    = postgres.NewStripRepository(nil)
	_ cdm.CdmStripStore                    = postgres.NewStripRepository(nil)
	_ cdm.CdmSequenceStripStore            = postgres.NewStripRepository(nil)
	_ ecfmp.StripStore                     = postgres.NewStripRepository(nil)
	_ frontend.FrontendStripUpdateStore    = postgres.NewStripRepository(nil)
	_ frontend.SnapshotStripStore          = postgres.NewStripRepository(nil)
	_ services.StripLifecycleStore         = postgres.NewStripRepository(nil)
	_ services.StripReader                 = postgres.NewStripRepository(nil)
	_ services.StripOrderingStore          = postgres.NewStripRepository(nil)
	_ services.StripFieldStore             = postgres.NewStripRepository(nil)
	_ services.StripOwnerStore             = postgres.NewStripRepository(nil)
	_ services.StripCdmStore               = postgres.NewStripRepository(nil)
	_ services.StripValidationStatusStore  = postgres.NewStripRepository(nil)
	_ services.StripManualFplStore         = postgres.NewStripRepository(nil)
	_ services.TrafficMetricsStripStore    = postgres.NewStripRepository(nil)
)
