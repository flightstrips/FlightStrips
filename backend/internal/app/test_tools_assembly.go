package app

import (
	appconfig "FlightStrips/internal/config"
	"FlightStrips/internal/euroscope"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testtools"
	"FlightStrips/internal/vatsim"
)

type testToolsAssembly struct {
	api *testtools.WebAPI
}

type testToolsAssemblyDependencies struct {
	auth                            shared.AuthenticationService
	readiness                       appconfig.StandAssignmentReadiness
	source                          *vatsim.SyntheticSource
	reconciler                      *vatsim.Reconciler
	sat                             satAssembly
	core                            coreRepositories
	stripDeleter                    testtools.StripDeleter
	clock                           *testtools.Clock
	euroscope                       *euroscope.Hub
	enableStandAssignmentESMessages bool
}

func assembleTestTools(enabled bool, deps testToolsAssemblyDependencies) testToolsAssembly {
	if !enabled {
		if deps.enableStandAssignmentESMessages && deps.sat.departures != nil {
			deps.sat.departures.SetWrongStandMessenger(deps.euroscope)
		}
		return testToolsAssembly{}
	}

	service := testtools.NewService(testtools.ServiceConfig{
		Source: deps.source, Reconciler: deps.reconciler,
		Departures: deps.sat.departures, Arrivals: deps.sat.arrivals,
		Allocations: deps.sat.allocations, Sessions: deps.core.sessions, Strips: deps.core.strips,
		StripDeleter: deps.stripDeleter,
		Assignments:  deps.sat.assignments, Stands: appconfig.GetStandCapabilities(), Clock: deps.clock,
	})
	if deps.enableStandAssignmentESMessages && deps.sat.departures != nil {
		deps.sat.departures.SetWrongStandMessenger(testtools.NewFallbackMessenger(deps.euroscope, service))
	}
	return testToolsAssembly{api: testtools.NewWebAPI(service, deps.auth, deps.readiness)}
}
