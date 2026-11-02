package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestDecodeAirportRejectsNonFiniteNumbers(t *testing.T) {
	for _, scalar := range []string{".nan", ".inf", "-.inf"} {
		t.Run(scalar, func(t *testing.T) {
			data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) { c["geography"].(map[string]any)["latitude"] = "REVIEW_NUMBER" })
			data = bytes.ReplaceAll(data, []byte("REVIEW_NUMBER"), []byte(scalar))
			_, err := DecodeAirport("selected.yaml", bytes.NewReader(data))
			if err == nil {
				t.Fatal("nonfinite number accepted")
			}
			for _, context := range []string{"selected.yaml airport EKCH", "$.geography.latitude", "line", "finite"} {
				if !strings.Contains(err.Error(), context) {
					t.Fatalf("missing %s: %v", context, err)
				}
			}
		})
	}
}

func TestDecodeAirportPreservesNumericSelectorValues(t *testing.T) {
	for _, scalar := range []string{"014", "0xC", "12.0", "12e0", "12"} {
		t.Run("stand_"+scalar, func(t *testing.T) {
			data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
				c["routing"].(map[string]any)["ground"].([]any)[0].(map[string]any)["forStandRanges"] = []any{"REVIEW_NUMBER"}
			})
			data = bytes.ReplaceAll(data, []byte("REVIEW_NUMBER"), []byte(scalar))
			doc, err := DecodeAirport("stand.yaml", bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			stand := doc.Routing.Ground[0].ForStandRanges[0]
			if stand.From != 12 || stand.To != 12 {
				t.Fatalf("numeric selector %s changed value: %+v", scalar, stand)
			}
		})
	}
	for _, scalar := range []string{"011", "0x9", "9.0", "9e0", "9"} {
		t.Run("runway_"+scalar, func(t *testing.T) {
			data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
				g := c["geography"].(map[string]any)
				g["runways"] = append(g["runways"].([]any), "09")
				c["routing"].(map[string]any)["ground"].([]any)[0].(map[string]any)["forRunway"] = "REVIEW_NUMBER"
			})
			data = bytes.ReplaceAll(data, []byte("REVIEW_NUMBER"), []byte(scalar))
			doc, err := DecodeAirport("runway.yaml", bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if doc.Routing.Ground[0].ForRunway != "09" {
				t.Fatalf("numeric runway %s became %s", scalar, doc.Routing.Ground[0].ForRunway)
			}
		})
	}
	data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
		c["routing"].(map[string]any)["ground"].([]any)[0].(map[string]any)["forStandRanges"] = []any{"014"}
	})
	doc, err := DecodeAirport("literal.yaml", bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Routing.Ground[0].ForStandRanges[0].From != 14 {
		t.Fatal("explicit string selector changed meaning")
	}
}

func TestDecodeAirportRejectsInvertedNumericStandBounds(t *testing.T) {
	for _, bounds := range [][2]string{{"30.0", "10.0"}, {"30e0", "10e0"}, {"9223372036854775808", "9223372036854775807"}} {
		data := mutateAirport(t, airportExample(t, "ekch"), func(c map[string]any) {
			c["routing"].(map[string]any)["ground"].([]any)[0].(map[string]any)["forStandRanges"] = []any{map[string]any{"prefix": "A", "from": "REVIEW_FROM", "to": "REVIEW_TO"}}
		})
		data = bytes.ReplaceAll(data, []byte("REVIEW_FROM"), []byte(bounds[0]))
		data = bytes.ReplaceAll(data, []byte("REVIEW_TO"), []byte(bounds[1]))
		_, err := DecodeAirport("inverted.yaml", bytes.NewReader(data))
		if err == nil || !strings.Contains(err.Error(), "stand range is inverted") {
			t.Fatalf("inverted numeric range accepted: %v", err)
		}
	}
}
