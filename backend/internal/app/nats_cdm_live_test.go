package app

import (
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	es "FlightStrips/pkg/events/euroscope"
)

func TestBuildNATSCDMCalculatesWithLiveEuroScopePositions(t *testing.T) {
	f := newRuntimeFixture(t, nil)
	master := f.socket(0, "111111", "EKCH_A_TWR", "118.100")
	f.sync(master, &es.Strip{Callsign: "SAS1", Origin: "EKCH", Destination: "EGLL", HasFp: true, AssignedSquawk: "1001", Runway: "22R", Sid: "ODN1C", Eobt: time.Now().UTC().Add(10 * time.Minute).Format("1504"), Position: &es.Position{Lat: 55.63, Lon: 12.65}})
	for i := 0; i < 50; i++ {
		f.send(master, &es.Envelope{Event: &es.Envelope_AircraftPositionUpdate{AircraftPositionUpdate: &es.AircraftPositionUpdateEvent{Callsign: "SAS1", Lat: 55.63, Lon: 12.65}}})
	}
	f.await("CDM sequence is calculated and replicated during live traffic", func() bool {
		for _, app := range f.apps {
			state, err := app.natsRuntime.projection.Read(sessionNATSRef(f.session))
			if err != nil {
				return false
			}
			cdm := state.Indexes[pb.EntityKind_CDM_STATE]["SAS1"].GetValue().GetCdmState()
			if cdm.GetTobt() == nil || cdm.GetTsat() == nil || cdm.GetTtot() == nil {
				return false
			}
		}
		return true
	})
}
