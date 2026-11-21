package operational

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/models"
	"FlightStrips/internal/vatsim"
	"github.com/stretchr/testify/require"
)

type vatsimPrefileTestSessions struct{}

func (vatsimPrefileTestSessions) List(context.Context) ([]*models.Session, error) {
	return []*models.Session{{ID: 1, Name: "LIVE", Airport: "EKCH"}}, nil
}

func TestLiveSessionUsesLaterPrefileThroughVATSIMFeed(t *testing.T) {
	for _, scenario := range []string{"prefile filed while previous leg was online", "lower-revision prefile after EuroScope-only departure"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := t.Context()
			now := time.Date(2026, 11, 9, 12, 5, 0, 0, time.UTC)
			filedAt := now.Add(-5 * time.Minute)
			repository := &memoryRepository{}
			deps := disconnectedExpiryDependencies(repository, &recordingPublisher{}, &now)
			deps.Sessions = testSessions{{ID: 1, Airport: "EKCH", Live: true}}
			service, err := New(deps)
			require.NoError(t, err)
			source := vatsim.NewSnapshotReplaySource()
			worker, err := vatsim.NewObservationWorker(vatsim.ObservationWorkerDependencies{
				Cache: source, Sessions: vatsimPrefileTestSessions{}, Sink: service, EnabledAirports: []string{"EKCH"},
				Now: func() time.Time { return now },
			})
			require.NoError(t, err)
			publish := func(pilots, prefiles []map[string]any) {
				t.Helper()
				data, err := json.Marshal(map[string]any{
					"general": map[string]any{"update_timestamp": now}, "pilots": pilots, "prefiles": prefiles,
				})
				require.NoError(t, err)
				require.NoError(t, source.Load(bytes.NewReader(data), now))
				require.NoError(t, worker.Publish(ctx))
				service.Reconcile(ctx)
			}
			flight := func(updatedAt time.Time, departure string, revision int) map[string]any {
				return map[string]any{
					"cid": 1234, "callsign": "SAS123", "last_updated": updatedAt,
					"flight_plan": map[string]any{
						"departure": "ESSA", "arrival": "EKCH", "deptime": departure, "revision_id": revision,
					},
				}
			}
			var nextPrefile map[string]any
			if scenario == "prefile filed while previous leg was online" {
				nextPrefile = flight(filedAt, "1400", 1)
				pilot := flight(now, "1100", 8)
				pilot["latitude"], pilot["longitude"], pilot["altitude"], pilot["groundspeed"] = 54, 11, 30000, 450
				pilot["logon_time"] = now.Add(-time.Hour)
				publish([]map[string]any{pilot}, []map[string]any{nextPrefile})
				now = now.Add(time.Minute)
			} else {
				publish(nil, []map[string]any{flight(filedAt, "1300", 8)})
				now = now.Add(30 * time.Minute)
				altitude, speed := 30000, 450.0
				require.NoError(t, service.Observe(aman.WithSession(ctx, 1), aman.FlightObservation{
					Callsign: "SAS123", Origin: "ESSA", Destination: "EKCH", Provider: aman.ObservationProviderEuroScope,
					SurveillanceSource: aman.SurveillanceSourceEuroScope, SourceStatus: aman.DataFresh, ReconciledAt: now,
					TakeoffDetected: timePointer(now), Surveillance: &aman.SurveillanceFact{LatitudeDegrees: 54, LongitudeDegrees: 11,
						AltitudeFeet: &altitude, GroundspeedKnots: &speed, ObservedAt: timePointer(now)},
				}))
				service.Reconcile(ctx)
				now = now.Add(aman.ArrivalSurveillanceFreshness + time.Second)
				nextPrefile = flight(now, "1400", 1)
			}
			previous := stateFlight(t, repository.state, "SAS123")
			require.Equal(t, aman.StateAirborne, previous.State)

			publish(nil, []map[string]any{nextPrefile})
			planned := stateFlight(t, repository.state, "SAS123")
			require.Equal(t, aman.StatePlanned, planned.State)
			require.NotEqual(t, previous.TelexEpisodeID(), planned.TelexEpisodeID())
			require.Equal(t, time.Date(2026, 11, 9, 14, 0, 0, 0, time.UTC), *planned.ConsumedPrefileDepartureAt)
			require.Nil(t, planned.LatestObservation.TakeoffDetected)
			require.Empty(t, repository.state.DisconnectedArrivals, "the prefile started the next leg before any disappearance timeout")
		})
	}
}
