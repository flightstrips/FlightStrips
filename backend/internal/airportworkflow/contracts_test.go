package airportworkflow

import (
	"FlightStrips/internal/config"
	"os"
	"strings"
	"testing"
)

func definition(t *testing.T) *config.AirportDefinition {
	t.Helper()
	data, err := os.ReadFile("../../config/airports/ekch.airport.yaml")
	if err != nil {
		t.Fatal(err)
	}
	r, err := config.NewAirportRegistry(config.ProductionAirportCatalog(), []config.AirportInput{{Source: "fixture", Reader: strings.NewReader(string(data))}})
	if err != nil {
		t.Fatal(err)
	}
	d, _ := r.Lookup("EKCH")
	return d
}

type contractHandler struct{}

func (contractHandler) Observe(Context, Observation) (Decision, error) { return Decision{}, nil }
func (contractHandler) Lifecycle(Context, Lifecycle) (LifecycleDecision, error) {
	return LifecycleDecision{Admit: true}, nil
}
func (contractHandler) Coordinate(Context, Coordination) (CoordinationDecision, error) {
	return CoordinationDecision{}, nil
}

func TestTypedActionsAndCompleteDecisionValidation(t *testing.T) {
	type custom struct{ Destination string }
	d := definition(t)
	b, err := Bind(d, Descriptor{ID: "ekch/v1", Bind: func(*config.AirportDefinition) (Handler, []ActionDescriptor, error) {
		return contractHandler{}, []ActionDescriptor{Action("custom.place", func(_ Context, a custom) (Decision, error) { return Decision{Bay: &a.Destination}, nil })}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	c := Context{Session: 7, Definition: d, Source: HumanSource}
	result, err := b.Action(c, "custom.place", custom{"TAXI_LWR"})
	if err != nil || result.Bay == nil || *result.Bay != "TAXI_LWR" {
		t.Fatalf("%+v %v", result, err)
	}
	for _, test := range []struct {
		name    string
		request any
	}{{"unknown", custom{}}, {"custom.place", Move{}}, {"custom.place", custom{"BOGUS"}}} {
		if _, err := b.Action(c, test.name, test.request); err == nil {
			t.Fatalf("accepted %+v", test)
		}
	}
	c.Source = ObservationSource
	if _, err := b.Action(c, "custom.place", custom{"TAXI"}); err == nil {
		t.Fatal("observation invoked action")
	}
	state := "TAXI"
	if err := b.Validate(c, Decision{GroundState: &state}); err == nil {
		t.Fatal("observation emitted outbound command")
	}
	c.Session = 0
	if _, err := b.Observe(c, Observation{}); err == nil {
		t.Fatal("accepted missing session")
	}
	if _, err := Bind(d, Descriptor{ID: "unknown"}); err == nil {
		t.Fatal("unknown implementation fell back")
	}
}

func TestInvalidActionRegistrationAndFieldIntent(t *testing.T) {
	d := definition(t)
	if _, err := Bind(d, Descriptor{ID: "ekch/v1", Bind: func(*config.AirportDefinition) (Handler, []ActionDescriptor, error) {
		return contractHandler{}, []ActionDescriptor{Action[Move]("nil", nil)}, nil
	}}); err == nil {
		t.Fatal("registered a nil action callback")
	}
	b, err := Bind(d, Descriptor{ID: "ekch/v1", Bind: func(*config.AirportDefinition) (Handler, []ActionDescriptor, error) {
		return contractHandler{}, nil, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	state := "BOGUS"
	if err := b.Validate(Context{Session: 1, Source: HumanSource}, Decision{GroundState: &state}); err == nil {
		t.Fatal("accepted invalid outbound state")
	}
}
