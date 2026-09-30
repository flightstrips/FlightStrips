package navigationcandidate

import (
	"context"
	"testing"
	"time"

	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/navdata/fixture"
	"FlightStrips/internal/aman/terminal"
)

func TestSourceMaterializesTypedAIRACPage(t *testing.T) {
	data := fixture.EKCH()
	provider := fixture.New(data)
	from, until, imported := data.Version.EffectiveFrom, data.Version.EffectiveUntil, data.Provenance.ImportedAt
	course := 221.2
	config := terminal.Configuration{
		SchemaVersion: terminal.SchemaVersion, ConfigVersion: "fixture-terminal", Airport: "EKCH",
		ApplicabilityFrom: from, ApplicabilityUntil: until,
		Dataset: terminal.DatasetCompatibility{Cycle: data.Version.Cycle, EffectiveFrom: from, EffectiveUntil: until},
		Sources: []terminal.Source{{ID: "fixture-aip", Document: "official fixture", EffectiveFrom: from, EffectiveUntil: until}},
		RunwayGroups: []terminal.RunwayGroup{{ID: "SOUTH", Runways: []navdata.RunwayID{"22L"}, FinalApproaches: []terminal.FinalApproachDefinition{{
			Runway: "22L", FinalApproachFix: "KEMAX", Threshold: terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: 55.6254111111, LongitudeDeg: 12.6675805556}, CourseTrueDeg: &course},
			CourseTrueDeg: course, PhysicalLengthM: 3302, Provenance: terminal.ProvenanceDefinition{SourceID: "fixture", SourceRevision: "fixture-r1", ImportedAt: imported, EffectiveFrom: from, EffectiveUntil: until},
		}}}},
		Feeders: []terminal.Feeder{{ID: "SOK"}},
		Paths:   []terminal.Path{{Feeder: "SOK", RunwayGroup: "SOUTH", Fixes: []navdata.FixID{"SOK", "KEMAX"}, MergeFix: "KEMAX", SelectedHolding: "SOK-HF"}},
	}
	source := Source{Cycles: provider, Airports: provider, Runways: provider, Procedures: provider, Fixes: provider, Routes: provider, Terminal: config, Now: func() time.Time { return from.Add(24 * time.Hour) }}
	page, checkpoint, err := source.Fetch(context.Background(), "EKCH", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if checkpoint.GetEtag() != data.Version.SourceRevision || len(page.GetFragments()) < 3 || provider.Calls() == 0 {
		t.Fatalf("incomplete materialized page: checkpoint=%v fragments=%d calls=%d", checkpoint, len(page.GetFragments()), provider.Calls())
	}
	kinds := map[string]int{}
	for _, fragment := range page.Fragments {
		switch {
		case fragment.GetAirportFragment() != nil:
			kinds["airport"]++
			if len(fragment.GetAirportFragment().GetRunways()) != 1 {
				t.Fatal("runway lost")
			}
		case fragment.GetProcedureFragment() != nil:
			kinds["procedure"]++
		case fragment.GetFixFragment() != nil:
			kinds["fix"]++
		case fragment.GetTerminalFragment() != nil:
			kinds["terminal"]++
			if len(fragment.GetTerminalFragment().GetPaths()) == 0 {
				t.Fatal("terminal path lost")
			}
		}
	}
	if kinds["airport"] != 1 || kinds["procedure"] == 0 || kinds["fix"] != 1 || kinds["terminal"] != 1 {
		t.Fatalf("fragment kinds: %v", kinds)
	}
}
