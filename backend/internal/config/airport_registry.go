package config

import (
	"FlightStrips/internal/vatsim"
	pkgmodels "FlightStrips/pkg/models"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"reflect"
	"slices"
	"strings"

	"go.yaml.in/yaml/v4"
)

// HandlerDescriptor registers compiled configuration metadata only. Runtime
// workflow decisions/execution are supplied by MA-02, not by configuration.
type HandlerDescriptor struct {
	ID             string
	View           string
	Layouts        []string
	RequiredBays   []string
	DecodeSettings func(*yaml.Node) (any, error)
	Validate       func(*AirportDocument) error
}
type AirportCatalog struct{ handlers map[string]HandlerDescriptor }

func NewAirportCatalog(descriptors ...HandlerDescriptor) (*AirportCatalog, error) {
	catalog := &AirportCatalog{handlers: map[string]HandlerDescriptor{}}
	for _, d := range descriptors {
		if d.ID == "" || d.View == "" || d.DecodeSettings == nil || d.Validate == nil {
			return nil, fmt.Errorf("incomplete compiled handler descriptor %q", d.ID)
		}
		if _, ok := catalog.handlers[d.ID]; ok {
			return nil, fmt.Errorf("duplicate handler descriptor %q", d.ID)
		}
		d.Layouts = slices.Clone(d.Layouts)
		d.RequiredBays = slices.Clone(d.RequiredBays)
		catalog.handlers[d.ID] = d
	}
	return catalog, nil
}

// TypedAirportSettings is a startup factory for concrete, closed settings.
func TypedAirportSettings[T any](node *yaml.Node) (any, error) {
	data, err := yaml.Marshal(node)
	if err != nil {
		return nil, err
	}
	var settings T
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&settings); err != nil {
		return nil, err
	}
	return settings, nil
}

func EKCHDescriptor() HandlerDescriptor {
	return HandlerDescriptor{
		ID: "ekch/v1", View: "ekch", Layouts: []string{"AMAN", "SEQPLN", "CLX", "AA", "AAAD", "AD", "GEGW", "TWTE", "TWRGND"},
		RequiredBays:   []string{"UNKNOWN", "NOT_CLEARED", "CLEARED", "PUSH", "TAXI", "TAXI_LWR", "TAXI_TWR", "DEPART", "AIRBORNE", "FINAL", "RWY_ARR", "TWY_ARR", "STAND", "HIDDEN", "HIDDEN_DEP", "DEP_HIDDEN", "ARR_HIDDEN", "CONTROLZONE"},
		DecodeSettings: TypedAirportSettings[EKCHSettings],
		Validate: func(d *AirportDocument) error {
			settings, ok := d.Workflow.Settings.(EKCHSettings)
			if !ok {
				return fmt.Errorf("workflow.settings: expected EKCHSettings")
			}
			if d.Airport != "EKCH" {
				return fmt.Errorf("workflow.handler: ekch/v1 requires EKCH")
			}
			if d.Ownership.Strips != "route" || d.Ownership.Sectors != "coverage" {
				return fmt.Errorf("ownership: EKCH requires route/coverage")
			}
			// The existing execution helpers still use these constants until MA-02.
			if d.Geography.Latitude != 55.6181 || d.Geography.Longitude != 12.656 || d.Geography.ElevationFT != 17 || settings.RelevantDistanceNM != 30 {
				return fmt.Errorf("geography/workflow.settings: EKCH execution requires its current geography and relevance distance")
			}
			return nil
		},
	}
}
func ProductionAirportCatalog() *AirportCatalog {
	catalog, err := NewAirportCatalog(EKCHDescriptor(), RADISDescriptor())
	if err != nil {
		panic(err)
	}
	return catalog
}

func (c *AirportCatalog) bind(doc *AirportDocument, settings *yaml.Node) error {
	d, ok := c.handlers[doc.Workflow.Handler]
	if !ok {
		return fmt.Errorf("workflow.handler: no compiled descriptor for %q", doc.Workflow.Handler)
	}
	if doc.View.ID != d.View {
		return fmt.Errorf("view.id: unsupported %q for handler %s", doc.View.ID, d.ID)
	}
	for _, variants := range doc.View.Layouts {
		for _, v := range variants {
			if !slices.Contains(d.Layouts, v.Layout) {
				return fmt.Errorf("view.layouts: unknown layout %q", v.Layout)
			}
		}
	}
	if !slices.Contains(d.Layouts, doc.View.AirborneFallbackLayout) {
		return fmt.Errorf("view.airborne_fallback_layout: unknown layout %q", doc.View.AirborneFallbackLayout)
	}
	for _, required := range d.RequiredBays {
		if !slices.ContainsFunc(doc.Workflow.Bays, func(b AirportBay) bool { return b.ID == required }) {
			return fmt.Errorf("workflow.bays: missing required bay %q", required)
		}
	}
	decoded, err := d.DecodeSettings(settings)
	if err != nil {
		return fmt.Errorf("workflow.settings: %w", err)
	}
	doc.Workflow.Settings = decoded
	return d.Validate(doc)
}

type AirportDefinition struct {
	textPilot    *pkgmodels.TextPilotInputs
	document     AirportDocument
	source       string
	revision     string
	resources    map[string]string
	services     map[string]bool
	regionGroups [3][]Region
}

func (d *AirportDefinition) Airport() string          { return d.document.Airport }
func (d *AirportDefinition) MembershipPolicy() string { return d.document.Coverage.Membership.Policy }
func (d *AirportDefinition) ViewID() string           { return d.document.View.ID }
func (d *AirportDefinition) Source() string           { return d.source }
func (d *AirportDefinition) Revision() string         { return d.revision }
func (d *AirportDefinition) Document() AirportDocument {
	return cloneAirportValue(reflect.ValueOf(d.document)).Interface().(AirportDocument)
}
func (d *AirportDefinition) DomainConfig() Config { return d.Document().DomainConfig() }

func (d *AirportDefinition) InitialCFLForRunway(runway string) (int, bool) {
	cfl, ok := d.document.Geography.InitialCFLFT[strings.ToUpper(strings.TrimSpace(runway))]
	return cfl, ok
}

func (d *AirportDefinition) Coordinates() (float64, float64) {
	return d.document.Geography.Latitude, d.document.Geography.Longitude
}

func (d *AirportDefinition) PreemptHigherPriority() bool {
	return d.document.Master.PreemptHigherPriority
}

// MasterPriority uses bound startup data and existing frequency normalization.
func (d *AirportDefinition) MasterPriority(callsign, frequency string) int {
	callsign = strings.ToUpper(strings.TrimSpace(callsign))
	frequency = vatsim.NormalizeFrequency(frequency)
	resolved := callsign
	for _, position := range d.document.Positions {
		if frequency != "" && vatsim.NormalizeFrequency(position.Frequency) == frequency {
			resolved = position.Name
			break
		}
	}
	for i, priority := range d.document.Master.Priorities {
		if priority.Match == "primary-frequency-or-callsign" {
			if callsign == priority.Position {
				return i
			}
			for _, position := range d.document.Positions {
				if position.Name == priority.Position && frequency != "" && vatsim.NormalizeFrequency(position.Frequency) == frequency {
					return i
				}
			}
		} else if resolved == priority.Position {
			return i
		}
	}
	return len(d.document.Master.Priorities)
}

// cloneAirportValue copies only decoded configuration values. Settings factories
// must return concrete data values, never service/client/runtime state.
func cloneAirportValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type()).Elem()
		out.Set(cloneAirportValue(value.Elem()))
		return out
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type().Elem())
		out.Elem().Set(cloneAirportValue(value.Elem()))
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneAirportValue(value.Index(i)))
		}
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), cloneAirportValue(iter.Value()))
		}
		return out
	case reflect.Struct:
		out := reflect.New(value.Type()).Elem()
		for i := 0; i < value.NumField(); i++ {
			out.Field(i).Set(cloneAirportValue(value.Field(i)))
		}
		return out
	default:
		return value
	}
}

type AirportRegistry struct{ definitions map[string]*AirportDefinition }
type AirportInput struct {
	Source string
	Reader io.Reader
}

// NewAirportRegistry atomically validates exactly the explicitly selected inputs.
// There are no mutation, discovery or reload methods on the resulting registry.
func NewAirportRegistry(catalog *AirportCatalog, selected []AirportInput) (*AirportRegistry, error) {
	if catalog == nil {
		return nil, fmt.Errorf("compiled airport catalog is required")
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("at least one explicit airport file must be selected")
	}
	registry := &AirportRegistry{definitions: map[string]*AirportDefinition{}}
	for _, input := range selected {
		doc, err := decodeAirport(input.Source, input.Reader, catalog)
		if err != nil {
			return nil, err
		}
		if _, ok := registry.definitions[doc.Airport]; ok {
			return nil, fmt.Errorf("%s airport %s: duplicate selected airport", input.Source, doc.Airport)
		}
		// Settings are already concrete; this is hashing, never event-time parsing.
		data, err := yaml.Marshal(doc)
		if err != nil {
			return nil, err
		}
		definition := &AirportDefinition{document: cloneAirportValue(reflect.ValueOf(*doc)).Interface().(AirportDocument), source: input.Source, revision: fmt.Sprintf("%x", sha256.Sum256(data))}
		registry.definitions[doc.Airport] = definition
	}
	return registry, nil
}
func (r *AirportRegistry) Lookup(airport string) (*AirportDefinition, bool) {
	if r == nil {
		return nil, false
	}
	definition, ok := r.definitions[airport]
	return definition, ok
}
func (r *AirportRegistry) Airports() []string {
	result := make([]string, 0, len(r.definitions))
	for airport := range r.definitions {
		result = append(result, airport)
	}
	slices.Sort(result)
	return result
}

func (d *AirportDefinition) PositionSection(frequency string) string {
	frequency = vatsim.NormalizeFrequency(frequency)
	for _, position := range d.document.Positions {
		if vatsim.NormalizeFrequency(position.Frequency) == frequency {
			return position.Section
		}
	}
	return ""
}

func (d *AirportDefinition) UsesStripOwnership() bool { return d.document.Ownership.Strips != "none" }

func (d *AirportDefinition) TextPilotInputs() *pkgmodels.TextPilotInputs {
	if d == nil || d.textPilot == nil {
		return nil
	}
	return cloneAirportValue(reflect.ValueOf(d.textPilot)).Interface().(*pkgmodels.TextPilotInputs)
}
