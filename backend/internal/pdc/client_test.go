package pdc

import (
	"reflect"
	"testing"
)

func TestParseResponse(t *testing.T) {
	c := NewClient("testlogon")

	t.Run("Nested messages from issue", func(t *testing.T) {
		input := "ok {NOZ938 telex {REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A}} {NOZ938 telex {REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A}}"

		expected := []Message{
			{
				From:   "NOZ938",
				To:     "telex",
				Type:   "telex",
				Packet: "REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A",
				Raw:    "NOZ938 telex {REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A}",
			},
			{
				From:   "NOZ938",
				To:     "telex",
				Type:   "telex",
				Packet: "REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A",
				Raw:    "NOZ938 telex {REQUEST PREDEP CLEARANCE NOZ938 B738 TO ENGM AT EKCH STAND A17 ATIS A}",
			},
		}

		got := c.parseResponse(input)

		if len(got) != len(expected) {
			t.Fatalf("expected %d messages, got %d", len(expected), len(got))
		}

		for i := range expected {
			if !reflect.DeepEqual(got[i], expected[i]) {
				t.Errorf("at index %d:\nexpected %+v\ngot %+v", i, expected[i], got[i])
			}
		}
	})
}
