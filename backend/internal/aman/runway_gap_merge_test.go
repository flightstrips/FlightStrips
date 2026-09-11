package aman

import (
	"encoding/json"
	"slices"
	"testing"
	"time"
)

func TestMergeRunwayGapUnionsSameRunwayIntervals(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	tests := map[string]struct {
		existing []RunwayGap
		start    time.Time
		end      time.Time
		want     RunwayGap
		IDs      []RunwayGapID
	}{
		"overlap": {
			existing: []RunwayGap{gapAt("old", base, 5, 15)}, start: base.Add(10 * time.Minute), end: base.Add(20 * time.Minute),
			want: gapAt("command", base, 5, 20), IDs: []RunwayGapID{"old"},
		},
		"touching": {
			existing: []RunwayGap{gapAt("old", base, 0, 10)}, start: base.Add(10 * time.Minute), end: base.Add(20 * time.Minute),
			want: gapAt("command", base, 0, 20), IDs: []RunwayGapID{"old"},
		},
		"containment": {
			existing: []RunwayGap{gapAt("old", base, 5, 25)}, start: base.Add(10 * time.Minute), end: base.Add(20 * time.Minute),
			want: gapAt("command", base, 5, 25), IDs: []RunwayGapID{"old"},
		},
		"multiple chain": {
			existing: []RunwayGap{gapAt("third", base, 20, 30), gapAt("first", base, 0, 10), gapAt("second", base, 12, 18)},
			start:    base.Add(10 * time.Minute), end: base.Add(20 * time.Minute),
			want: gapAt("command", base, 0, 30), IDs: []RunwayGapID{"first", "second", "third"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result, err := MergeRunwayGap([]RunwayGroupPolicy{{ID: "north", Gaps: test.existing}}, mergeInput(base, test.start, test.end))
			if err != nil {
				t.Fatalf("merge runway gap: %v", err)
			}
			test.want.Label, test.want.CreatedAt, test.want.CreatedBy = "new reason", base, "new-controller"
			if result.Union != test.want {
				t.Fatalf("union = %#v, want %#v", result.Union, test.want)
			}
			if !slices.Equal(result.ReplacedIDs, test.IDs) {
				t.Fatalf("replaced IDs = %v, want %v", result.ReplacedIDs, test.IDs)
			}
			if len(result.RunwayGroups[0].Gaps) != 1 || result.RunwayGroups[0].Gaps[0] != test.want {
				t.Fatalf("persisted gaps = %#v, want only %#v", result.RunwayGroups[0].Gaps, test.want)
			}
		})
	}
}

func TestMergeRunwayGapKeepsDisjointAndOtherRunwayIntervals(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	north := []RunwayGap{gapAt("earlier", base, 0, 5), gapAt("later", base, 25, 30)}
	south := []RunwayGap{gapAt("same-time-other-runway", base, 10, 20)}
	groups := []RunwayGroupPolicy{{ID: "north", Gaps: north}, {ID: "south", Gaps: south}}

	result, err := MergeRunwayGap(groups, mergeInput(base, base.Add(10*time.Minute), base.Add(20*time.Minute)))
	if err != nil {
		t.Fatalf("merge runway gap: %v", err)
	}
	if len(result.ReplacedIDs) != 0 || len(result.RunwayGroups[0].Gaps) != 3 {
		t.Fatalf("disjoint gaps were replaced: %#v", result)
	}
	if !slices.Equal(result.RunwayGroups[1].Gaps, south) {
		t.Fatalf("other runway changed: %#v", result.RunwayGroups[1].Gaps)
	}
	if &result.RunwayGroups[0].Gaps[0] == &north[0] || &result.RunwayGroups[1].Gaps[0] == &south[0] {
		t.Fatal("merge result aliases input gap storage")
	}
}

func TestMergeRunwayGapIsDeterministicAndUsesAcceptedMetadata(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	gaps := []RunwayGap{
		gapWithMetadata("late", base, 18, 25, "late reason", "old-b"),
		gapWithMetadata("early", base, 5, 12, "early reason", "old-a"),
	}
	input := mergeInput(base, base.Add(10*time.Minute), base.Add(20*time.Minute))

	first, err := MergeRunwayGap([]RunwayGroupPolicy{{ID: "north", Gaps: gaps}}, input)
	if err != nil {
		t.Fatalf("first merge: %v", err)
	}
	second, err := MergeRunwayGap([]RunwayGroupPolicy{{ID: "north", Gaps: slices.Clone(gaps)}}, input)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	slices.Reverse(gaps)
	third, err := MergeRunwayGap([]RunwayGroupPolicy{{ID: "north", Gaps: gaps}}, input)
	if err != nil {
		t.Fatalf("reversed merge: %v", err)
	}
	firstJSON, _ := json.Marshal(first)
	secondJSON, _ := json.Marshal(second)
	thirdJSON, _ := json.Marshal(third)
	if string(firstJSON) != string(secondJSON) || string(firstJSON) != string(thirdJSON) {
		t.Fatalf("merge is not deterministic:\n%s\n%s\n%s", firstJSON, secondJSON, thirdJSON)
	}
	if first.Union.ID != "command" || first.Union.Label != "new reason" || first.Union.CreatedAt != base || first.Union.CreatedBy != "new-controller" {
		t.Fatalf("union metadata = %#v", first.Union)
	}
	if !slices.Equal(first.ReplacedIDs, []RunwayGapID{"early", "late"}) {
		t.Fatalf("replaced IDs = %v", first.ReplacedIDs)
	}
}

func TestMergedRunwayGapReplayRetainsOnlyCanonicalUnion(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	groups := []RunwayGroupPolicy{{ID: "north", Gaps: []RunwayGap{
		gapAt("first", base, 0, 10), gapAt("second", base, 20, 30),
	}}}
	merged, err := MergeRunwayGap(groups, mergeInput(base, base.Add(10*time.Minute), base.Add(20*time.Minute)))
	if err != nil {
		t.Fatalf("merge runway gap: %v", err)
	}
	state := AirportState{Airport: "EKCH", GeneratedAt: base, PolicyVersion: "gap-union-v1", Mode: ModeReadOnly, RunwayGroups: merged.RunwayGroups}
	if err := state.Validate(); err != nil {
		t.Fatalf("validate merged state: %v", err)
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal merged state: %v", err)
	}
	var replayed AirportState
	if err := json.Unmarshal(encoded, &replayed); err != nil {
		t.Fatalf("unmarshal merged state: %v", err)
	}
	if err := replayed.Validate(); err != nil {
		t.Fatalf("validate replayed state: %v", err)
	}
	if len(replayed.RunwayGroups[0].Gaps) != 1 || replayed.RunwayGroups[0].Gaps[0].ID != "command" {
		t.Fatalf("replayed gaps contain hidden fragments: %#v", replayed.RunwayGroups[0].Gaps)
	}
	reencoded, _ := json.Marshal(replayed)
	if string(encoded) != string(reencoded) {
		t.Fatalf("replay changed canonical bytes:\n%s\n%s", encoded, reencoded)
	}
}

func TestMergeRunwayGapRejectsMissingGroupAndReusedCommandID(t *testing.T) {
	base := time.Date(2026, time.September, 11, 12, 0, 0, 0, time.UTC)
	input := mergeInput(base, base.Add(10*time.Minute), base.Add(20*time.Minute))
	for name, groups := range map[string][]RunwayGroupPolicy{
		"missing group":     {{ID: "south"}},
		"reused command ID": {{ID: "north"}, {ID: "south", Gaps: []RunwayGap{gapAt("command", base, 30, 40)}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := MergeRunwayGap(groups, input); err == nil {
				t.Fatal("expected invalid merge")
			}
		})
	}
}

func gapAt(id RunwayGapID, base time.Time, startMinute, endMinute int) RunwayGap {
	return gapWithMetadata(id, base, startMinute, endMinute, "old reason", "old-controller")
}

func gapWithMetadata(id RunwayGapID, base time.Time, startMinute, endMinute int, label, creator string) RunwayGap {
	return RunwayGap{ID: id, Start: base.Add(time.Duration(startMinute) * time.Minute), End: base.Add(time.Duration(endMinute) * time.Minute), Label: label, CreatedAt: base.Add(-time.Hour), CreatedBy: creator}
}

func mergeInput(createdAt, start, end time.Time) RunwayGapMergeInput {
	return RunwayGapMergeInput{RunwayGroupID: "north", CommandID: "command", Interval: RunwayGapInterval{start: start, end: end}, Label: "new reason", CreatedAt: createdAt, CreatedBy: "new-controller"}
}
