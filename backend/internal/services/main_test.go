package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/testutil"
	"FlightStrips/pkg/events/euroscope"
	"context"
	"os"
	"testing"
)

var testWorkflows *airportworkflow.Registry

func newWorkflowTestService(reader StripReader, options ...StripServiceOption) *StripService {
	service := NewStripService(reader, append([]StripServiceOption{WithAirportWorkflows(testWorkflows)}, options...)...)
	b, _ := testWorkflows.Lookup("EKCH")
	fixture, err := airportworkflow.Bind(config.AirportDefinitionWithTestGeometry(b.Definition()), airportworkflow.EKCHDescriptor())
	if err != nil {
		panic(err)
	}
	service.sessionWorkflows.Store(int32(1), fixture)
	if service.sessionRepo == nil {
		service.workflowSessions = &testutil.MockSessionRepository{GetByIDFn: func(_ context.Context, id int32) (*internalModels.Session, error) {
			return &internalModels.Session{ID: id, Airport: "EKCH"}, nil
		}}
	}
	return service
}

func TestMain(m *testing.M) {
	if err := os.Chdir("../.."); err != nil {
		panic("failed to chdir to backend root: " + err.Error())
	}

	if err := config.InitEKCHTestConfig(); err != nil {
		panic("failed to initialize config: " + err.Error())
	}
	definitions, err := config.LoadAirportFiles(config.ProductionAirportCatalog(), []string{"backend/config/airports/ekch.airport.yaml"}, config.AirportLoadOptions{RepositoryRoot: ".."})
	if err != nil {
		panic(err)
	}
	testWorkflows, err = airportworkflow.NewRegistry(definitions, airportworkflow.EKCHDescriptor())
	if err != nil {
		panic(err)
	}
	code := m.Run()
	if err := testdata.ShutdownTestDB(); err != nil && code == 0 {
		code = 1
	}
	os.Exit(code)
}

func shouldRestartStripLifecycle(existing *internalModels.Strip, incoming euroscope.Strip, airport string) bool {
	b, _ := testWorkflows.Lookup(airport)
	d, err := b.Lifecycle(airportworkflow.Context{Session: 1, Strip: workflowSnapshot(existing)}, airportworkflow.Lifecycle{Origin: incoming.Origin, Destination: incoming.Destination, Latitude: incoming.GetPosition().GetLat(), Longitude: incoming.GetPosition().GetLon(), Altitude: incoming.GetPosition().GetAltitude()})
	return err == nil && d.Restart
}
