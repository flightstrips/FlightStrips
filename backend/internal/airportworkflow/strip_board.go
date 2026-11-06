package airportworkflow

import (
	"FlightStrips/pkg/models"
	"fmt"
	"reflect"
)

// These are shared semantic actions. Each compiled handler registers only the
// actions it supports and returns decisions; shared services own all I/O/state.
type StripAnnotation struct {
	Field   string `json:"field"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
}
type StripVisibility struct {
	Visible bool
	Reason  string
}
type StripAdmission struct {
	Bay         string
	Annotations models.StripAnnotations
}
type ConnectedFlightSearch struct{ Query string }
type ConnectedFlightLookup struct{ Airport, Query string }
type StripRemoval struct{}
type StripRestoration struct{}
type FlightPlanCreation struct{}

func validateDeclaredAnnotations(before, after models.StripAnnotations, declared map[string]string) error {
	a, b := reflect.ValueOf(before), reflect.ValueOf(after)
	typ := a.Type()
	for i := 0; i < a.NumField(); i++ {
		if reflect.DeepEqual(a.Field(i).Interface(), b.Field(i).Interface()) {
			continue
		}
		field := typ.Field(i)
		name := field.Tag.Get("json")
		want := "string"
		if field.Type.Kind() == reflect.Bool {
			want = "boolean"
		}
		if field.Type.Kind() == reflect.Pointer {
			want = "timestamp"
		}
		if declared[name] != want {
			return fmt.Errorf("annotation %s is not declared as %s", name, want)
		}
	}
	return nil
}
