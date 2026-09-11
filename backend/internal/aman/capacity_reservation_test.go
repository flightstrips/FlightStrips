package aman

import (
	"crypto/sha256"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestCapacityReservationDefaultsLabelAndDerivesIDFromCommand(t *testing.T) {
	now := time.Date(2026, time.September, 12, 10, 0, 0, 0, time.UTC)
	reservation, err := NewRunwayCapacityReservation(
		"command-42", now.Add(time.Hour), now.Add(63*time.Minute), "", now, "controller-1",
	)
	if err != nil {
		t.Fatalf("create capacity reservation: %v", err)
	}
	if reservation.ID != "command-42" {
		t.Errorf("reservation ID = %q, want command-derived ID", reservation.ID)
	}
	if reservation.Label != DefaultCapacityReservationLabel {
		t.Errorf("reservation label = %q, want %q", reservation.Label, DefaultCapacityReservationLabel)
	}
}

func TestCapacityReservationValidatesCanonicalAbsoluteInterval(t *testing.T) {
	now := time.Date(2026, time.September, 12, 10, 0, 0, 0, time.UTC)
	valid := RunwayCapacityReservation{
		ID: "command-42", Start: now.Add(time.Hour), End: now.Add(63 * time.Minute),
		Label: "FLIGHT", CreatedAt: now, CreatedBy: "controller-1",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("validate reservation: %v", err)
	}

	for name, mutate := range map[string]func(*RunwayCapacityReservation){
		"empty ID":         func(value *RunwayCapacityReservation) { value.ID = "" },
		"unclean ID":       func(value *RunwayCapacityReservation) { value.ID = " command-42 " },
		"empty label":      func(value *RunwayCapacityReservation) { value.Label = "" },
		"unclean label":    func(value *RunwayCapacityReservation) { value.Label = " FLIGHT " },
		"empty creator":    func(value *RunwayCapacityReservation) { value.CreatedBy = "" },
		"zero start":       func(value *RunwayCapacityReservation) { value.Start = time.Time{} },
		"empty interval":   func(value *RunwayCapacityReservation) { value.End = value.Start },
		"reverse interval": func(value *RunwayCapacityReservation) { value.End = value.Start.Add(-time.Second) },
		"non-UTC start": func(value *RunwayCapacityReservation) {
			value.Start = value.Start.In(time.FixedZone("CEST", 2*60*60))
		},
		"non-UTC end": func(value *RunwayCapacityReservation) {
			value.End = value.End.In(time.FixedZone("CEST", 2*60*60))
		},
		"non-UTC creation": func(value *RunwayCapacityReservation) {
			value.CreatedAt = value.CreatedAt.In(time.FixedZone("CEST", 2*60*60))
		},
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			assertInvalidArgument(t, candidate.Validate())
		})
	}
}

func TestCapacityReservationJSONIsAdditiveCanonicalAndDigestStable(t *testing.T) {
	var legacy RunwayGroupPolicy
	if err := json.Unmarshal([]byte(`{"ID":"north","Selected":true}`), &legacy); err != nil {
		t.Fatalf("decode legacy runway group: %v", err)
	}
	if legacy.CapacityReservations != nil {
		t.Fatalf("legacy reservations = %#v, want nil", legacy.CapacityReservations)
	}

	now := time.Date(2026, time.September, 12, 10, 0, 0, 0, time.UTC)
	first := RunwayCapacityReservation{ID: "command-1", Start: now.Add(time.Hour), End: now.Add(63 * time.Minute), Label: "FLIGHT", CreatedAt: now, CreatedBy: "controller-1"}
	second := RunwayCapacityReservation{ID: "command-2", Start: first.Start, End: now.Add(66 * time.Minute), Label: "TRAINING", CreatedAt: now.Add(time.Minute), CreatedBy: "controller-2"}
	state := AirportState{
		Airport: "EKCH", GeneratedAt: now, PolicyVersion: "reservation-v1", Mode: ModeReadOnly,
		RunwayGroups: []RunwayGroupPolicy{{ID: "north", CapacityReservations: []RunwayCapacityReservation{first, second}}},
	}
	if err := state.Validate(); err != nil {
		t.Fatalf("validate capacity reservations: %v", err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal capacity reservations: %v", err)
	}
	var restored AirportState
	if err := json.Unmarshal(encoded, &restored); err != nil {
		t.Fatalf("restore capacity reservations: %v", err)
	}
	if err := restored.Validate(); err != nil {
		t.Fatalf("validate restored capacity reservations: %v", err)
	}
	replayed, err := json.Marshal(restored)
	if err != nil {
		t.Fatalf("marshal restored capacity reservations: %v", err)
	}
	if string(encoded) != string(replayed) || sha256.Sum256(encoded) != sha256.Sum256(replayed) {
		t.Fatalf("capacity reservation replay changed canonical bytes:\nfirst:  %s\nsecond: %s", encoded, replayed)
	}
	if len(restored.Flights) != 0 {
		t.Fatalf("capacity reservation entered flight collection: %#v", restored.Flights)
	}

	reordered := state
	reordered.RunwayGroups = append([]RunwayGroupPolicy(nil), state.RunwayGroups...)
	reordered.RunwayGroups[0].CapacityReservations = []RunwayCapacityReservation{second, first}
	assertInvalidArgument(t, reordered.Validate())
	duplicate := state
	duplicate.RunwayGroups = append(duplicate.RunwayGroups, RunwayGroupPolicy{ID: "south", CapacityReservations: []RunwayCapacityReservation{first}})
	assertInvalidArgument(t, duplicate.Validate())
}

func TestCapacityReservationContainsOnlyCapacityAndAuditFields(t *testing.T) {
	want := []string{"ID", "Start", "End", "Label", "CreatedAt", "CreatedBy"}
	typeOf := reflect.TypeFor[RunwayCapacityReservation]()
	if typeOf.NumField() != len(want) {
		t.Fatalf("capacity reservation fields = %d, want %d", typeOf.NumField(), len(want))
	}
	for index, name := range want {
		field := typeOf.Field(index)
		if field.Name != name {
			t.Errorf("capacity reservation field %d = %q, want %q", index, field.Name, name)
		}
		if tag := field.Tag.Get("json"); tag != "" {
			t.Errorf("capacity reservation field %q declares wire JSON tag %q", field.Name, tag)
		}
	}
}
