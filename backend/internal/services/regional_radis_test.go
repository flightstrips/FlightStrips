package services

import (
	"FlightStrips/internal/airportworkflow"
	"FlightStrips/internal/config"
	"FlightStrips/internal/database"
	"FlightStrips/internal/models"
	"FlightStrips/internal/pdc/testdata"
	"FlightStrips/internal/repository/postgres"
	"FlightStrips/internal/shared"
	"FlightStrips/internal/testutil"
	"FlightStrips/internal/vatsim"
	es "FlightStrips/pkg/events/euroscope"
	pkg "FlightStrips/pkg/models"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRegionalRADISPersistsATDAndManualAdmissionWithRealDefinitions(t *testing.T) {
	pool, q := testdata.SetupTestDB(t)
	ctx := t.Context()
	definitions, err := config.LoadAirportFiles(config.ProductionAirportCatalog(), []string{"backend/config/airports/ekbi.airport.yaml", "backend/config/airports/ekyt.airport.yaml"}, config.AirportLoadOptions{RepositoryRoot: "..", LookupEnv: func(string) (string, bool) { return "backend/config/test/ICAO_Aircraft.json", true }})
	require.NoError(t, err)
	workflows, err := airportworkflow.NewRegistry(definitions, airportworkflow.RADISDescriptor())
	require.NoError(t, err)
	repo := postgres.NewStripRepository(pool)
	sessions := postgres.NewSessionRepository(pool)
	commander := &testutil.MockEuroscopeHub{}
	newService := func() *StripService {
		return NewStripService(repo, WithAirportWorkflows(workflows), WithSessionReader(sessions), WithStripEventPublisher(&productionWorkflowPublisher{}), WithControllerReader(postgres.NewControllerRepository(pool)), WithSectorOwnerRepository(postgres.NewSectorOwnerRepository(pool)), WithCoordinationStore(postgres.NewCoordinationRepository(pool)), WithEuroscopeCommander(commander))
	}
	s := newService()
	now := time.Now().UTC().Add(time.Second)
	for _, airport := range []string{"EKBI", "EKYT"} {
		t.Run(airport, func(t *testing.T) {
			require.NoError(t, q.InsertAirport(ctx, airport))
			session, err := q.InsertSession(ctx, database.InsertSessionParams{Airport: airport, Name: "LIVE"})
			require.NoError(t, err)
			definition, _ := definitions.Lookup(airport)
			geo := definition.Document().Geography
			binding := &testutil.MockCommandBinding{SessionID: session, CIDValue: "OWN"}
			own, err := shared.WithHumanCommand(ctx, binding)
			require.NoError(t, err)
			source := vatsim.NewSyntheticSource()
			s.SetConnectedFlightSource(source, time.Minute, func() time.Time { return now })
			unassociated := es.Strip{Callsign: "NOFP", Origin: airport, Destination: "EGLL", Runway: geo.Runways[0], Position: &es.Position{Lat: geo.Latitude, Lon: geo.Longitude}}
			commandsBefore := len(commander.ClearedAltitudes)
			require.NoError(t, s.SyncStrip(ctx, session, "MASTER", unassociated, airport))
			require.Len(t, commander.ClearedAltitudes, commandsBefore, "no automatic ES amendment without a flight plan")
			unassociated.HasFp = true
			require.NoError(t, s.SyncStrip(ctx, session, "MASTER", unassociated, airport))
			associated, err := repo.GetByCallsign(ctx, session, "NOFP")
			require.NoError(t, err)
			require.Equal(t, int32(geo.InitialCFLFT[unassociated.Runway]), *associated.ClearedAltitude, "first real flight-plan association receives initial clearance")
			vfr := unassociated
			vfr.Callsign, vfr.Destination, vfr.FlightRules = "CIRCUIT", airport, "V"
			commandsBefore = len(commander.ClearedAltitudes)
			require.NoError(t, s.SyncStrip(ctx, session, "MASTER", vfr, airport))
			require.Len(t, commander.ClearedAltitudes, commandsBefore, "local VFR circuit must not receive an IFR FL60/FL120 clearance")
			for _, departure := range []bool{true, false} {
				callsign, origin, destination := "ARR1", "EGLL", airport
				if departure {
					callsign, origin, destination = "DEP1", airport, "EGLL"
				}
				read := func() *models.Strip {
					v, err := repo.GetByCallsign(ctx, session, callsign)
					require.NoError(t, err)
					return v
				}
				event := es.Strip{Callsign: callsign, Origin: origin, Destination: destination, Runway: geo.Runways[0], HasFp: true, Position: &es.Position{Lat: geo.Latitude, Lon: geo.Longitude, Altitude: int32(geo.ElevationFT) + 499}}
				if !departure {
					event.ClearedAltitude = 9000
				}
				require.NoError(t, s.SyncStrip(ctx, session, "MASTER", event, airport))
				if departure {
					require.Equal(t, int32(geo.InitialCFLFT[event.Runway]), *read().ClearedAltitude)
					require.Equal(t, int32(geo.InitialCFLFT[event.Runway]), commander.ClearedAltitudes[len(commander.ClearedAltitudes)-1].Altitude)
				}
				require.Empty(t, read().BoardState.Annotations.ATD)
				require.NoError(t, s.UpdateAircraftPosition(ctx, session, callsign, geo.Latitude, geo.Longitude, int32(geo.ElevationFT)+500, airport))
				require.Equal(t, now.Format("1504"), read().BoardState.Annotations.ATD)
				require.Nil(t, read().BoardState.Annotations.ATA)
				now = now.Add(time.Minute)
				require.NoError(t, s.UpdateAircraftPosition(ctx, session, callsign, 0, 0, 40000, airport))
				require.Equal(t, now.Add(-time.Minute).Format("1504"), read().BoardState.Annotations.ATD)
				require.Nil(t, read().BoardState.RemovedAt)
				current := read()
				revision := pkg.StripRevision{StripID: current.ID, Version: current.Version, Definition: definition.Revision()}
				require.NoError(t, s.AnnotateStrip(own, session, callsign, "OWN", airport, "", revision, airportworkflow.StripAnnotation{Field: "atd", Value: ""}))
				restarted := newService()
				restarted.SetConnectedFlightSource(source, time.Minute, func() time.Time { return now })
				require.NoError(t, restarted.UpdateAircraftPosition(ctx, session, callsign, geo.Latitude, geo.Longitude, 1000, airport))
				require.Empty(t, read().BoardState.Annotations.ATD)
				require.True(t, read().BoardState.ATDManual)
				// FIND also pins a previously automatic, already-associated strip.
				source.Upsert(vatsim.Flight{Callsign: callsign, CID: "123", State: vatsim.FlightStateOnline, LogonTime: now, FlightPlan: vatsim.FlightPlan{Origin: origin, Destination: destination, Revision: 1}})
				s.SetConnectedFlightSource(source, time.Hour, func() time.Time { return now })
				flights, err := s.FindConnectedFlights(own, session, "OWN", airport, definition.Revision(), callsign)
				require.NoError(t, err)
				require.Len(t, flights, 1)
				require.NoError(t, s.AdmitConnectedFlight(own, session, "OWN", airport, definition.Revision(), flights[0]))
				require.True(t, read().BoardState.ManualAdmission)
				require.True(t, read().BoardState.ATDManual)
				require.NoError(t, s.DeleteStrip(ctx, session, callsign))
				require.Nil(t, read().BoardState.RemovedAt, "FIND strip survives aircraft disconnect")
			}
			delay, override, err := s.AircraftDisconnectDelay(ctx, session)
			require.NoError(t, err)
			require.True(t, override)
			require.Equal(t, time.Minute, delay)
			require.Empty(t, binding.Sent(), "paper annotations and FIND do not issue ES commands")
			require.False(t, definition.ServiceEnabled("cdm"))
		})
	}
}
