// Package airportworkflow contains pure, compiled airport workflow decisions.
// It owns no repository, transport, worker or mutable session state.
package airportworkflow

import (
	"FlightStrips/internal/config"
	"FlightStrips/pkg/events/euroscope"
	pkgmodels "FlightStrips/pkg/models"
	"fmt"
	"reflect"
	"sort"
	"time"
)

// Source distinguishes reconciliation from a new command. Authority is supplied
// by the authenticated executor, never selected by a handler.
type Source uint8

const (
	ObservationSource Source = iota
	HumanSource
	AutomaticSource
)

// Snapshot contains values only. It cannot alias a stored strip or its owners.
type Snapshot struct {
	RemovedAt                                              *time.Time
	RemovalReason                                          string
	BoardMember                                            bool
	Local                                                  bool
	HasFP                                                  bool
	Annotations                                            pkgmodels.StripAnnotations
	GroundStateKnown                                       bool
	Runway                                                 string
	Landed                                                 bool
	EuroscopeSeen                                          bool
	Exists                                                 bool
	ID, Version                                            int32
	Callsign, Origin, Destination, Bay, GroundState, Owner string
	AssignedSquawk, PDCState, TrackingController           string
	Cleared, StartRequested                                bool
	Latitude, Longitude                                    float64
	Altitude                                               int32
}

type Context struct {
	Now          time.Time
	Session      int32
	Definition   *config.AirportDefinition
	Source       Source
	Strip        Snapshot
	GroundOnline bool
}

type ObservationKind uint8

const (
	FullFlight ObservationKind = iota
	GroundState
	Position
	ArrivalPosition
	ClearedFlag
	BackendState
)

// Observation is normalized by the ingress path. Missing full-flight route
// fields are resolved before decision evaluation by the production sync path.
type Observation struct {
	Kind                             ObservationKind
	Origin, Destination, GroundState string
	Cleared                          bool
	Latitude, Longitude              float64
	Altitude                         int32
}

type LifecycleKind uint8

const (
	Admission LifecycleKind = iota
	StandExpiry
	Removal
	BackendAdmission
)

type Lifecycle struct {
	Reason                           string
	Kind                             LifecycleKind
	Latitude, Longitude              float64
	Altitude                         int32
	Origin, Destination, GroundState string
	Cleared                          bool
}

type LifecycleDecision struct {
	Visibility             *StripVisibility
	RemovalDelay           *time.Duration
	Admit, Restart, Remove bool
	Bay                    *string
	StandRemovalDelay      time.Duration
}

type Coordination struct {
	Sequence                     bool
	Coverage                     map[string][]string
	From, To                     string
	TargetIsTower                bool
	SQPosition, DeliveryPosition string
}

type CoordinationDecision struct {
	SequenceSupported                bool
	SequenceOwner, SequenceFrequency string
	ClearanceOwner                   string
	PromoteTaxi, Handover            bool
}

// Decision requests existing typed move/field operations. Observation decisions
// contain reconciliation only; they cannot produce outbound field commands.
type Decision struct {
	Admission                         *StripAdmission
	ConnectedFlights                  *ConnectedFlightLookup
	Visibility                        *StripVisibility
	Annotations                       *pkgmodels.StripAnnotations
	AutoAssume, ConfirmVoiceClearance bool
	Ignore                            bool
	ReconciledGroundState             *string
	ReconciledCleared                 *bool
	Touchdown                         bool
	Bay                               *string
	GroundState                       *string
	Cleared                           *bool
}

type Clearance struct{ Cleared bool }

type Move struct {
	Bay                         string
	Clearance, ConfirmedRemoval bool
}

type Handler interface {
	Observe(Context, Observation) (Decision, error)
	Lifecycle(Context, Lifecycle) (LifecycleDecision, error)
	Coordinate(Context, Coordination) (CoordinationDecision, error)
}

// ActionDescriptor closes an action over its concrete Go request type. Custom
// actions register alongside their handler; the dispatcher has no name switch.
type ActionDescriptor struct {
	name    string
	request reflect.Type
	decide  func(Context, any) (Decision, error)
}

func Action[T any](name string, decide func(Context, T) (Decision, error)) ActionDescriptor {
	if decide == nil {
		return ActionDescriptor{}
	}
	return ActionDescriptor{name: name, request: reflect.TypeFor[T](), decide: func(c Context, request any) (Decision, error) {
		value, ok := request.(T)
		if !ok {
			return Decision{}, fmt.Errorf("action %s: expected %v, got %T", name, reflect.TypeFor[T](), request)
		}
		return decide(c, value)
	}}
}

type BoardStatePolicy struct {
	Revisioned      bool
	RecoverySeconds int
}

type Descriptor struct {
	BoardState func(*config.AirportDefinition) BoardStatePolicy
	ID         string
	Bind       func(*config.AirportDefinition) (Handler, []ActionDescriptor, error)
}

// Bound is immutable and shared across sessions. Binding occurs at startup,
// never while processing an event or holding a session/strip lock.
type Bound struct {
	boardState  BoardStatePolicy
	annotations map[string]string
	definition  *config.AirportDefinition
	handler     Handler
	actions     map[string]ActionDescriptor
	bays        map[string]bool
}

func Bind(d *config.AirportDefinition, descriptor Descriptor) (*Bound, error) {
	if d == nil || descriptor.ID == "" || descriptor.Bind == nil || descriptor.ID != d.Document().Workflow.Handler {
		return nil, fmt.Errorf("workflow implementation does not match definition")
	}
	handler, actions, err := descriptor.Bind(d)
	if err != nil {
		return nil, err
	}
	if handler == nil {
		return nil, fmt.Errorf("handler %s is nil", descriptor.ID)
	}
	b := &Bound{definition: d, handler: handler, actions: map[string]ActionDescriptor{}, bays: map[string]bool{}}
	if descriptor.BoardState != nil {
		b.boardState = descriptor.BoardState(d)
	}
	b.annotations = map[string]string{}
	for name, annotation := range d.Document().Workflow.Annotations {
		b.annotations[name] = annotation.Type
	}
	for _, bay := range d.Document().Workflow.Bays {
		b.bays[bay.ID] = true
	}
	for _, action := range actions {
		if action.name == "" || action.request == nil || action.decide == nil {
			return nil, fmt.Errorf("incomplete action descriptor")
		}
		if _, duplicate := b.actions[action.name]; duplicate {
			return nil, fmt.Errorf("duplicate action %s", action.name)
		}
		b.actions[action.name] = action
	}
	return b, nil
}

// ActionCatalog projects the same bound descriptors used by the executor.
// The returned slice cannot mutate the registry.
func (b *Bound) ActionCatalog() []string {
	names := make([]string, 0, len(b.actions))
	for name := range b.actions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
func (b *Bound) BoardStatePolicy() BoardStatePolicy    { return b.boardState }
func (b *Bound) Definition() *config.AirportDefinition { return b.definition }
func (b *Bound) HasBay(bay string) bool                { return b.bays[bay] }
func (b *Bound) context(c Context) (Context, error) {
	if c.Session <= 0 || (c.Definition != nil && c.Definition != b.definition) {
		return c, fmt.Errorf("workflow scope mismatch")
	}
	c.Definition = b.definition
	return c, nil
}
func (b *Bound) Validate(c Context, d Decision) error {
	if d.Visibility != nil && !b.boardState.Revisioned {
		return fmt.Errorf("invalid visibility decision")
	}
	if d.Annotations != nil && (c.Source == ObservationSource || !b.boardState.Revisioned) {
		return fmt.Errorf("invalid annotation decision")
	}
	if d.Annotations != nil {
		if err := validateDeclaredAnnotations(c.Strip.Annotations, *d.Annotations, b.annotations); err != nil {
			return err
		}
	}
	if d.Admission != nil && (!b.boardState.Revisioned || !b.HasBay(d.Admission.Bay)) {
		return fmt.Errorf("invalid board admission decision")
	}
	if d.Admission != nil {
		if err := validateDeclaredAnnotations(pkgmodels.StripAnnotations{}, d.Admission.Annotations, b.annotations); err != nil {
			return err
		}
	}
	if d.ConnectedFlights != nil && d.ConnectedFlights.Airport != b.definition.Airport() {
		return fmt.Errorf("connected lookup must retain airport scope")
	}
	if d.GroundState != nil {
		switch *d.GroundState {
		case euroscope.GroundStateUnknown, euroscope.GroundStateStartup, euroscope.GroundStatePush, euroscope.GroundStateTaxi, euroscope.GroundStateLineup, euroscope.GroundStateDepart, euroscope.GroundStateParked:
		default:
			return fmt.Errorf("workflow returned invalid ground state %q", *d.GroundState)
		}
	}
	if c.Source > AutomaticSource {
		return fmt.Errorf("unknown workflow source")
	}
	if d.GroundState != nil && d.Cleared != nil {
		return fmt.Errorf("workflow cannot combine ground-state and clearance operations")
	}
	if c.Source != ObservationSource && (d.Touchdown || d.ReconciledGroundState != nil || d.ReconciledCleared != nil) {
		return fmt.Errorf("controller action cannot masquerade as observation reconciliation")
	}
	if _, err := b.context(c); err != nil {
		return err
	}
	if d.Bay != nil && !b.HasBay(*d.Bay) {
		return fmt.Errorf("workflow returned unknown bay %q", *d.Bay)
	}
	if c.Source == ObservationSource && (d.GroundState != nil || d.Cleared != nil) {
		return fmt.Errorf("observation cannot emit field commands")
	}
	return nil
}
func (b *Bound) Action(c Context, name string, request any) (Decision, error) {
	c, err := b.context(c)
	if err != nil {
		return Decision{}, err
	}
	if c.Source == ObservationSource {
		return Decision{}, fmt.Errorf("observation cannot invoke controller action")
	}
	a, ok := b.actions[name]
	if !ok {
		return Decision{}, fmt.Errorf("unsupported action %q", name)
	}
	d, err := a.decide(c, request)
	if err != nil {
		return Decision{}, err
	}
	return d, b.Validate(c, d)
}
func (b *Bound) Observe(c Context, o Observation) (Decision, error) {
	c, err := b.context(c)
	if err != nil {
		return Decision{}, err
	}
	c.Source = ObservationSource
	d, err := b.handler.Observe(c, o)
	if err != nil {
		return Decision{}, err
	}
	return d, b.Validate(c, d)
}
func (b *Bound) Lifecycle(c Context, l Lifecycle) (LifecycleDecision, error) {
	c, err := b.context(c)
	if err != nil {
		return LifecycleDecision{}, err
	}
	d, err := b.handler.Lifecycle(c, l)
	if err == nil && d.Visibility != nil {
		err = b.Validate(c, Decision{Visibility: d.Visibility})
	}
	return d, err
}
func (b *Bound) Coordinate(c Context, event Coordination) (CoordinationDecision, error) {
	c, err := b.context(c)
	if err != nil {
		return CoordinationDecision{}, err
	}
	return b.handler.Coordinate(c, event)
}

type Registry struct{ airports map[string]*Bound }

func NewRegistry(definitions *config.AirportRegistry, descriptors ...Descriptor) (*Registry, error) {
	compiled := map[string]Descriptor{}
	for _, descriptor := range descriptors {
		if _, exists := compiled[descriptor.ID]; exists {
			return nil, fmt.Errorf("duplicate workflow %s", descriptor.ID)
		}
		compiled[descriptor.ID] = descriptor
	}
	r := &Registry{airports: map[string]*Bound{}}
	if definitions == nil {
		return r, nil
	}
	for _, airport := range definitions.Airports() {
		d, _ := definitions.Lookup(airport)
		b, err := Bind(d, compiled[d.Document().Workflow.Handler])
		if err != nil {
			return nil, fmt.Errorf("airport %s: %w", airport, err)
		}
		r.airports[airport] = b
	}
	return r, nil
}
func (r *Registry) Lookup(airport string) (*Bound, bool) {
	if r == nil {
		return nil, false
	}
	b, ok := r.airports[airport]
	return b, ok
}
