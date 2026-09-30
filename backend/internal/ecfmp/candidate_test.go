package ecfmp

import (
	"encoding/json"
	"testing"
	"time"

	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/proto"
)

func TestTypedPageRoundTripsMatcherInputs(t *testing.T) {
	now := time.Date(2026, 9, 30, 7, 0, 0, 0, time.UTC)
	measure := FlowMeasure{ID: 42, Ident: "EU42", StartTime: now, EndTime: now.Add(time.Hour),
		Measure: FlowMeasureType{Type: MeasureTypeMandatoryRoute, Value: json.RawMessage(`["DCT ABC","DCT XYZ"]`)},
		Filters: []FlowMeasureFilter{{Type: FilterTypeADEP, Value: json.RawMessage(`["EKCH"]`)}, {Type: FilterTypeLevelAbove, Value: json.RawMessage(`250`)}}}
	page, err := TypedPage([]FlowMeasure{measure}, now)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := proto.Marshal(&pb.ProviderPage{Provider: "ecfmp", Resource: "flow-measure/active", Parsed: &pb.ProviderPage_Ecfmp{Ecfmp: page}})
	if err != nil {
		t.Fatal(err)
	}
	decoded := &pb.ProviderPage{}
	if err := pb.UnmarshalStrict(encoded, decoded); err != nil {
		t.Fatal(err)
	}
	back, err := MeasuresFromPage(decoded.GetEcfmp())
	if err != nil || len(back) != 1 || back[0].ID != measure.ID || len(back[0].Measure.MandatoryRoutes()) != 2 || back[0].Filters[1].LevelValue() == nil || *back[0].Filters[1].LevelValue() != 250 {
		t.Fatalf("typed ECFMP round trip: %+v, %v", back, err)
	}
}

func TestTypedPageRejectsUnsupportedProviderShape(t *testing.T) {
	now := time.Now().UTC()
	measure := FlowMeasure{ID: 1, StartTime: now, EndTime: now.Add(time.Hour), Measure: FlowMeasureType{Type: MeasureTypeMandatoryRoute, Value: json.RawMessage(`{"route":"DCT"}`)}}
	if _, err := TypedPage([]FlowMeasure{measure}, now); err == nil {
		t.Fatal("untyped route object accepted")
	}
	measure.Measure.Value = json.RawMessage(`["DCT"]`)
	measure.Filters = []FlowMeasureFilter{{Type: "unknown", Value: json.RawMessage(`1`)}}
	if _, err := TypedPage([]FlowMeasure{measure}, now); err == nil {
		t.Fatal("unknown filter accepted")
	}
}
