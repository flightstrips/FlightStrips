package navigationcandidate

import (
	"fmt"
	"math"
	"sort"
	"time"

	"FlightStrips/internal/aman/navdata"
	pb "FlightStrips/pkg/events/cluster"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func candidatePage(airport string, c *candidateCollector) (*pb.AiracPage, error) {
	if c == nil || c.airport == nil || c.fixes == nil || c.terminal == nil || c.manifest == nil || string(c.manifest.Airport) != airport {
		return nil, fmt.Errorf("incomplete AIRAC candidate")
	}
	page := &pb.AiracPage{}
	a, err := airportData(*c.airport)
	if err != nil {
		return nil, err
	}
	page.Fragments = append(page.Fragments, a)
	procedures := append([]navdata.CandidateProcedureFragment(nil), c.procedures...)
	sort.Slice(procedures, func(i, j int) bool { return procedures[i].Kind < procedures[j].Kind })
	for _, fragment := range procedures {
		value, err := procedureData(fragment)
		if err != nil {
			return nil, err
		}
		page.Fragments = append(page.Fragments, value)
	}
	fix, err := fixData(airport, *c.fixes)
	if err != nil {
		return nil, err
	}
	page.Fragments = append(page.Fragments, fix)
	terminal, err := terminalData(*c.terminal)
	if err != nil {
		return nil, err
	}
	page.Fragments = append(page.Fragments, terminal)
	return page, nil
}

func version(v navdata.DatasetVersion) *pb.NavDatasetVersion {
	return &pb.NavDatasetVersion{Cycle: v.Cycle, SourceRevision: v.SourceRevision, EffectiveFrom: timestamppb.New(v.EffectiveFrom), EffectiveUntil: timestamppb.New(v.EffectiveUntil)}
}
func provenance(p navdata.Provenance) *pb.NavProvenance {
	return &pb.NavProvenance{SourceId: p.SourceID, SourceRevision: p.SourceRevision, ImportedAt: timestamppb.New(p.ImportedAt), EffectiveFrom: timestamppb.New(p.EffectiveFrom), EffectiveUntil: timestamppb.New(p.EffectiveUntil)}
}
func coordinate(c navdata.Coordinate) *pb.NavCoordinate {
	return &pb.NavCoordinate{LatitudeDegrees: c.LatitudeDeg, LongitudeDegrees: c.LongitudeDeg}
}
func optionalTime(t *time.Time) *timestamppb.Timestamp {
	if t == nil {
		return nil
	}
	return timestamppb.New(*t)
}
func base(airport string, v navdata.DatasetVersion, p navdata.Provenance, imported time.Time, validated *time.Time, state navdata.ValidationState, digest, schema string) *pb.NavData {
	return &pb.NavData{Airport: airport, Version: version(v), SchemaVersion: schema, Provenance: provenance(p), ImportedAt: timestamppb.New(imported), ValidatedAt: optionalTime(validated), ValidationState: string(state), Digest: digest}
}
func optionalInt(value *int) (*int32, error) {
	if value == nil {
		return nil, nil
	}
	if *value < math.MinInt32 || *value > math.MaxInt32 {
		return nil, fmt.Errorf("navigation integer exceeds int32")
	}
	v := int32(*value)
	return &v, nil
}
func threshold(t navdata.Threshold) (*pb.NavThreshold, error) {
	e, err := optionalInt(t.ElevationFt)
	if err != nil {
		return nil, err
	}
	return &pb.NavThreshold{Position: coordinate(t.Position), ElevationFeet: e, CourseTrueDegrees: t.CourseTrueDeg}, nil
}

func airportData(f navdata.CandidateAirportFragment) (*pb.NavData, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	value := base(string(f.Airport.ID), f.Version, f.Provenance, f.ImportedAt, f.ValidatedAt, f.State, f.Digest, f.SchemaVersion)
	fragment := &pb.NavAirportFragment{Airport: &pb.NavAirport{Icao: string(f.Airport.ID), Name: f.Airport.Name, Position: coordinate(f.Airport.Position), Provenance: provenance(f.Airport.Provenance)}}
	for _, runway := range f.Runways {
		t, err := threshold(runway.Threshold)
		if err != nil {
			return nil, err
		}
		fragment.Runways = append(fragment.Runways, &pb.NavRunway{Id: string(runway.ID), Airport: string(runway.Airport), Threshold: t, LengthNm: runway.LengthNM, Provenance: provenance(runway.Provenance)})
	}
	value.Fragment = &pb.NavData_AirportFragment{AirportFragment: fragment}
	return value, nil
}
func optionalFix(value *navdata.FixID) *string {
	if value == nil {
		return nil
	}
	text := string(*value)
	return &text
}
func optionalHolding(value *navdata.HoldingID) *string {
	if value == nil {
		return nil
	}
	text := string(*value)
	return &text
}
func leg(v navdata.ProcedureLeg) *pb.NavLeg {
	result := &pb.NavLeg{Id: v.ID, PathTerminator: string(v.PathTerminator), FromFix: optionalFix(v.FromFix), ToFix: optionalFix(v.ToFix), CourseTrueDegrees: v.CourseTrueDeg, DistanceNm: v.DistanceNM, HoldingId: optionalHolding(v.HoldingID)}
	if v.FromPosition != nil {
		result.FromPosition = coordinate(*v.FromPosition)
	}
	if v.ToPosition != nil {
		result.ToPosition = coordinate(*v.ToPosition)
	}
	return result
}
func holding(v navdata.HoldingPattern) (*pb.NavHolding, error) {
	minimum, err := optionalInt(v.MinimumAltitudeFt)
	if err != nil {
		return nil, err
	}
	maximum, err := optionalInt(v.MaximumAltitudeFt)
	if err != nil {
		return nil, err
	}
	speed, err := optionalInt(v.MaximumSpeedKt)
	if err != nil {
		return nil, err
	}
	result := &pb.NavHolding{Id: string(v.ID), Fix: string(v.Fix), InboundCourseTrueDegrees: v.InboundCourseTrueDeg, TurnDirection: string(v.TurnDirection), MinimumAltitudeFeet: minimum, MaximumAltitudeFeet: maximum, MaximumSpeedKnots: speed, Termination: string(v.Termination), Provenance: provenance(v.Provenance)}
	if v.LegLengthNM != nil {
		result.LegExtent = &pb.NavHolding_LengthNm{LengthNm: *v.LegLengthNM}
	}
	if v.LegTimeSeconds != nil {
		result.LegExtent = &pb.NavHolding_TimeSeconds{TimeSeconds: *v.LegTimeSeconds}
	}
	return result, nil
}
func procedureData(f navdata.CandidateProcedureFragment) (*pb.NavData, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	value := base(string(f.Airport), f.Version, f.Provenance, f.ImportedAt, f.ValidatedAt, f.State, f.Digest, f.SchemaVersion)
	fragment := &pb.NavProcedureFragment{Airport: string(f.Airport), Kind: string(f.Kind), Coverage: string(f.Coverage)}
	for _, p := range f.Procedures {
		item := &pb.NavProcedure{Id: string(p.ID), Airport: string(p.Airport), Kind: string(p.Kind), Provenance: provenance(p.Provenance)}
		for _, runway := range p.Runways {
			item.Runways = append(item.Runways, string(runway))
		}
		for _, entry := range p.Legs {
			item.Legs = append(item.Legs, leg(entry))
		}
		for _, entry := range p.Holdings {
			converted, err := holding(entry)
			if err != nil {
				return nil, err
			}
			item.Holdings = append(item.Holdings, converted)
		}
		fragment.Procedures = append(fragment.Procedures, item)
	}
	value.Fragment = &pb.NavData_ProcedureFragment{ProcedureFragment: fragment}
	return value, nil
}
func fixData(airport string, f navdata.CandidateFixFragment) (*pb.NavData, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	value := base(airport, f.Version, f.Provenance, f.ImportedAt, f.ValidatedAt, f.State, f.Digest, f.SchemaVersion)
	fragment := &pb.NavFixFragment{Coverage: string(f.Coverage)}
	for _, entry := range f.Fixes {
		fragment.Fixes = append(fragment.Fixes, &pb.NavFix{Id: string(entry.ID), Position: coordinate(entry.Position), Provenance: provenance(entry.Provenance)})
	}
	value.Fragment = &pb.NavData_FixFragment{FixFragment: fragment}
	return value, nil
}
func terminalData(f navdata.CandidateTerminalFragment) (*pb.NavData, error) {
	if err := f.Validate(); err != nil {
		return nil, err
	}
	value := base(string(f.Airport), f.Version, f.Provenance, f.ImportedAt, f.ValidatedAt, f.State, f.Digest, f.SchemaVersion)
	fragment := &pb.NavTerminalFragment{Airport: string(f.Airport), ConfigVersion: f.ConfigVersion}
	for _, policy := range f.STARFamilyPolicies {
		spacing := policy.SameSTARSpacing
		fragment.StarFamilyPolicies = append(fragment.StarFamilyPolicies, &pb.NavStarFamilyPolicy{StarFamily: string(policy.STARFamily), SameStarSpacing: &pb.AmanSameStarSpacing{Enabled: spacing.Enabled, ActivationRatePerHour: spacing.ActivationRatePerHour, MinimumEmptySlots: spacing.MinimumEmptySlots}, HoldingSequencePolicy: string(policy.HoldingSequencePolicy)})
	}
	for _, mapping := range f.TimelineMappings {
		item := &pb.AmanTimelineMapping{Id: uint32(mapping.ID)}
		if mapping.Left != nil {
			left := string(*mapping.Left)
			item.Left = &left
		}
		if mapping.Right != nil {
			right := string(*mapping.Right)
			item.Right = &right
		}
		fragment.TimelineMappings = append(fragment.TimelineMappings, item)
	}
	for _, path := range f.Paths {
		family := path.STARFamily
		if family == "" {
			family = navdata.STARFamilyID(path.Feeder)
		}
		item := &pb.NavTerminalPath{Version: version(path.Version), Airport: string(path.Airport), StarFamily: string(family), FeederFix: string(path.FeederFix), RunwayGroup: string(path.RunwayGroup), Coverage: string(path.Coverage), Unresolved: append([]string(nil), path.Unresolved...), Provenance: provenance(path.Provenance), Digest: path.Digest}
		if item.FeederFix == "" {
			item.FeederFix = string(path.Feeder)
		}
		if path.HoldingToFeederDuration != nil {
			item.HoldingToFeederDuration = durationpb.New(*path.HoldingToFeederDuration)
		}
		if path.PublishedHeadingMagneticDeg != nil {
			heading, err := optionalInt(path.PublishedHeadingMagneticDeg)
			if err != nil {
				return nil, err
			}
			item.PublishedHeadingMagneticDegrees = heading
		}
		for _, entry := range path.Legs {
			item.Legs = append(item.Legs, leg(entry))
		}
		for _, id := range path.HoldingIDs {
			item.HoldingIds = append(item.HoldingIds, string(id))
		}
		fragment.Paths = append(fragment.Paths, item)
	}
	for _, entry := range f.Holdings {
		converted, err := holding(entry)
		if err != nil {
			return nil, err
		}
		fragment.Holdings = append(fragment.Holdings, converted)
	}
	value.Fragment = &pb.NavData_TerminalFragment{TerminalFragment: fragment}
	return value, nil
}
