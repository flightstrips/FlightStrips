package cluster

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/cdm"
	pb "FlightStrips/pkg/events/cluster"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

type viffReadStub struct {
	flights int
	masters int
	airport string
}

func (s *viffReadStub) icao() string {
	if s.airport != "" {
		return s.airport
	}
	return "EKCH"
}

func (s *viffReadStub) IFPSByDepartureAirport(context.Context, string) (cdm.BulkIFPSData, error) {
	s.flights++
	return cdm.BulkIFPSData{{Callsign: "SAS123", Departure: s.icao(), Arrival: "EDDF", EOBT: "1200", Taxi: 10, CDMData: cdm.CDMData{TOBT: "1210", TSAT: "1220", Reason: "42"}}}, nil
}
func (s *viffReadStub) IFPSByCallsignParsed(context.Context, string) (*cdm.IFPSData, error) {
	s.flights++
	return &cdm.IFPSData{Callsign: "SAS123", Departure: s.icao(), Arrival: "EDDF"}, nil
}
func (s *viffReadStub) AirportMasters(context.Context) ([]cdm.AirportMaster, error) {
	s.masters++
	return []cdm.AirportMaster{{ICAO: s.icao(), Position: s.icao() + "_TWR"}}, nil
}

func TestTypedViffFlightPageRetainsCDMFields(t *testing.T) {
	rows := cdm.BulkIFPSData{{Callsign: "SAS123", CID: "123", Departure: "EKCH", Arrival: "EDDF", EOBT: "1200", TOBT: "1210", ReqTOBT: "1215", Taxi: 12, CTOT: "1300", AOBT: "1220", ATOT: "1305", ETA: "1400", MostPenalizingAirspace: "EDGG", CDMStatus: "READY", ATFCMStatus: "ACTIVE", CDMData: cdm.CDMData{TOBT: "1210", TSAT: "1225", TTOT: "1250", CTOT: "1300", Reason: "42", ReqTOBT: "1215", ReqTOBTType: "VIFF", ReqASRT: "1220", ID: "viff-id"}}}
	page, err := typedViffFlights("EKCH", rows, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	got := page.Flights[0]
	if got.Callsign != rows[0].Callsign || got.TaxiMinutes != 12 || got.CdmData.RequestedTobtType != "VIFF" || got.CdmData.Id != "viff-id" || got.MostPenalizingAirspace != "EDGG" {
		t.Fatalf("vIFF fields lost: %v", got)
	}
	rows[0].Departure = "ESSA"
	if _, err := typedViffFlights("EKCH", rows, time.Now().UTC()); err == nil {
		t.Fatal("foreign vIFF row accepted")
	}
}

func TestViffMastersTypedAirportCheckpoint(t *testing.T) {
	store, objects, nav := navFixture(t)
	ctx := context.Background()
	client := &viffReadStub{}
	adapter := ViffReadAdapter{State: nav, Worker: ExternalCallWorker{Writer: nav.Writer}, Client: client}
	deadline := time.Now().UTC()
	if sent, err := adapter.Masters(ctx, "EKCH", deadline); !sent || err != nil || client.masters != 1 {
		t.Fatalf("vIFF masters fetch: %v %v", sent, err)
	}
	page, revision, err := adapter.ReadMasters(ctx, "EKCH")
	if err != nil || page == nil || revision != 1 || len(page.Masters) != 1 || page.Masters[0].Position != "EKCH_TWR" {
		t.Fatalf("typed masters: %v revision=%d err=%v", page, revision, err)
	}
	handoffAirport(t, store)
	otherNav := NavigationWeather{Writer: Writer{Store: store, NodeID: "node-b"}, Objects: objects}
	other := ViffReadAdapter{State: otherNav, Worker: ExternalCallWorker{Writer: otherNav.Writer}, Client: client}
	if err := other.Resume(ctx, "EKCH", 0); err != nil {
		t.Fatal(err)
	}
	if sent, err := other.Masters(ctx, "EKCH", deadline); sent || err != nil || client.masters != 1 {
		t.Fatalf("vIFF masters replay: %v %v", sent, err)
	}
}

func TestViffFlightsSessionCheckpoint(t *testing.T) {
	store, objects, nav := navFixture(t)
	ref := sessionRef(123)
	subject, _ := Subject(ref)
	claim := &pb.StateEvent{SchemaVersion: 1, EventId: uuid.NewString(), Aggregate: ref, Actor: &pb.Actor{Kind: pb.Actor_SYSTEM, Id: "node-a"}, Fact: &pb.StateEvent_OwnerClaimed{OwnerClaimed: &pb.OwnerTerm{NodeId: "node-a", Epoch: 1}}}
	data, _ := proto.Marshal(claim)
	if _, err := store.Publish(context.Background(), subject, 0, data); err != nil {
		t.Fatal(err)
	}
	seedViffSession(t, nav.Writer, 123, "EKCH")
	client := &viffReadStub{}
	adapter := ViffReadAdapter{State: nav, Worker: ExternalCallWorker{Writer: nav.Writer}, Client: client}
	deadline := time.Now().UTC()
	if sent, err := adapter.Flights(context.Background(), "EKCH", 123, deadline); !sent || err != nil || client.flights != 1 {
		t.Fatalf("session vIFF read: %v %v calls=%d", sent, err, client.flights)
	}
	page, revision, err := adapter.ReadFlights(context.Background(), 123, "")
	if err != nil || page == nil || revision != 1 || len(page.Flights) != 1 {
		t.Fatalf("session vIFF checkpoint: %v revision=%d err=%v", page, revision, err)
	}
	handoffOwner(t, store, ref)
	otherNav := NavigationWeather{Writer: Writer{Store: store, NodeID: "node-b"}, Objects: objects}
	other := ViffReadAdapter{State: otherNav, Worker: ExternalCallWorker{Writer: otherNav.Writer}, Client: client}
	if err := other.Resume(context.Background(), "EKCH", 123); err != nil {
		t.Fatal(err)
	}
	if sent, err := other.Flights(context.Background(), "EKCH", 123, deadline); sent || err != nil || client.flights != 1 {
		t.Fatalf("session vIFF replay: %v %v calls=%d", sent, err, client.flights)
	}
}
