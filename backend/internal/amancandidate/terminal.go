package amancandidate

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
	pb "FlightStrips/pkg/events/cluster"
)

// TerminalPolicy is the typed import boundary for the operational settings in
// a verified NavTerminalFragment. Task 20's import assembly uses it alongside
// the existing geometry fragment; runtime evaluation reads only the accepted
// fragment. Source documents and validation metadata remain on NavData.
func TerminalPolicy(c terminal.Configuration, volume ...terminal.TMAVolume) *pb.NavTerminalPolicy {
	p := &pb.NavTerminalPolicy{Airport: string(c.Airport), ConfigVersion: c.ConfigVersion}
	if len(volume) > 0 {
		p.TmaVolume = encodeTMAVolume(volume[0])
	}
	for _, g := range c.RunwayGroups {
		r := &pb.NavPolicyRunwayGroup{Id: string(g.ID)}
		for _, a := range g.Aliases {
			r.Aliases = append(r.Aliases, string(a))
		}
		for _, a := range g.Runways {
			r.Runways = append(r.Runways, string(a))
		}
		if g.SameSTARSpacing != nil {
			s := g.SameSTARSpacing
			r.SameStarSpacing = &pb.AmanSameStarSpacing{Enabled: s.Enabled, ActivationRatePerHour: s.ActivationRatePerHour, MinimumEmptySlots: s.MinimumEmptySlots}
		}
		for _, a := range g.FinalApproaches {
			t := a.Threshold
			r.FinalApproaches = append(r.FinalApproaches, &pb.NavPolicyFinalApproach{Runway: string(a.Runway), FinalApproachFix: string(a.FinalApproachFix), CourseTrueDegrees: a.CourseTrueDeg, PhysicalLengthM: a.PhysicalLengthM,
				Threshold:  &pb.NavThreshold{Position: &pb.NavCoordinate{LatitudeDegrees: t.Position.LatitudeDeg, LongitudeDegrees: t.Position.LongitudeDeg}, ElevationFeet: optionalNumber[int32](t.ElevationFt), CourseTrueDegrees: t.CourseTrueDeg},
				Provenance: &pb.NavProvenance{SourceId: a.Provenance.SourceID, SourceRevision: a.Provenance.SourceRevision, ImportedAt: timestamp(a.Provenance.ImportedAt), EffectiveFrom: timestamp(a.Provenance.EffectiveFrom), EffectiveUntil: timestamp(a.Provenance.EffectiveUntil)}})
		}
		p.RunwayGroups = append(p.RunwayGroups, r)
	}
	for _, f := range c.Feeders {
		r := &pb.NavPolicyFeeder{Id: string(f.ID)}
		for _, a := range f.Aliases {
			r.Aliases = append(r.Aliases, string(a))
		}
		p.Feeders = append(p.Feeders, r)
	}
	for _, f := range c.Paths {
		r := &pb.NavPolicyPath{Feeder: string(f.Feeder), StarFamily: string(f.STARFamily), FeederFix: string(f.FeederFix), HoldingToFeederSeconds: f.HoldingToFeederSeconds, RunwayGroup: string(f.RunwayGroup), MergeFix: string(f.MergeFix), SelectedHolding: string(f.SelectedHolding), PublishedHeadingMagneticDegrees: optionalNumber[int32](f.PublishedHeadingMagneticDeg)}
		for _, a := range f.Fixes {
			r.Fixes = append(r.Fixes, string(a))
		}
		p.Paths = append(p.Paths, r)
	}
	for _, f := range c.STARFamilyPolicies {
		s := f.SameSTARSpacing
		p.StarFamilyPolicies = append(p.StarFamilyPolicies, &pb.NavStarFamilyPolicy{StarFamily: string(f.STARFamily), HoldingSequencePolicy: string(f.HoldingSequencePolicy), SameStarSpacing: &pb.AmanSameStarSpacing{Enabled: s.Enabled, ActivationRatePerHour: s.ActivationRatePerHour, MinimumEmptySlots: s.MinimumEmptySlots}})
	}
	return p
}

func encodeTMAVolume(volume terminal.TMAVolume) *pb.NavTmaVolume {
	v := &pb.NavTmaVolume{}
	for _, poly := range volume.Coordinates() {
		p := &pb.NavTmaPolygon{}
		for _, ring := range poly {
			r := &pb.NavTmaRing{}
			for _, c := range ring {
				r.Coordinates = append(r.Coordinates, &pb.NavCoordinate{LongitudeDegrees: c[0], LatitudeDegrees: c[1]})
			}
			p.Rings = append(p.Rings, r)
		}
		v.Polygons = append(v.Polygons, p)
	}
	return v
}
func decodeTMAVolume(volume *pb.NavTmaVolume) (terminal.TMAVolume, error) {
	var result [][][][]float64
	for _, poly := range volume.Polygons {
		var p [][][]float64
		for _, ring := range poly.Rings {
			var r [][]float64
			for _, c := range ring.Coordinates {
				r = append(r, []float64{c.LongitudeDegrees, c.LatitudeDegrees})
			}
			p = append(p, r)
		}
		result = append(result, p)
	}
	return terminal.TMAVolumeFromCoordinates(result)
}

func decodeTerminalPolicy(p *pb.NavTerminalPolicy) terminal.Configuration {
	c := terminal.Configuration{Airport: navdata.AirportID(p.Airport), ConfigVersion: p.ConfigVersion}
	for _, g := range p.RunwayGroups {
		r := terminal.RunwayGroup{ID: aman.RunwayGroupID(g.Id)}
		for _, a := range g.Aliases {
			r.Aliases = append(r.Aliases, aman.RunwayGroupID(a))
		}
		for _, a := range g.Runways {
			r.Runways = append(r.Runways, navdata.RunwayID(a))
		}
		if s := g.SameStarSpacing; s != nil {
			r.SameSTARSpacing = &terminal.SameSTARSpacing{Enabled: s.Enabled, ActivationRatePerHour: s.ActivationRatePerHour, MinimumEmptySlots: s.MinimumEmptySlots}
		}
		for _, a := range g.FinalApproaches {
			t := a.GetThreshold()
			if t == nil {
				t = &pb.NavThreshold{}
			}
			pos := t.GetPosition()
			v := a.GetProvenance()
			r.FinalApproaches = append(r.FinalApproaches, terminal.FinalApproachDefinition{Runway: navdata.RunwayID(a.Runway), FinalApproachFix: navdata.FixID(a.FinalApproachFix), CourseTrueDeg: a.CourseTrueDegrees, PhysicalLengthM: a.PhysicalLengthM,
				Threshold:  terminal.ThresholdDefinition{Position: terminal.CoordinateDefinition{LatitudeDeg: pos.GetLatitudeDegrees(), LongitudeDeg: pos.GetLongitudeDegrees()}, ElevationFt: optionalNumber[int](t.ElevationFeet), CourseTrueDeg: t.CourseTrueDegrees},
				Provenance: terminal.ProvenanceDefinition{SourceID: v.GetSourceId(), SourceRevision: v.GetSourceRevision(), ImportedAt: instant(v.GetImportedAt()), EffectiveFrom: instant(v.GetEffectiveFrom()), EffectiveUntil: instant(v.GetEffectiveUntil())}})
		}
		c.RunwayGroups = append(c.RunwayGroups, r)
	}
	for _, f := range p.Feeders {
		r := terminal.Feeder{ID: navdata.FeederID(f.Id)}
		for _, a := range f.Aliases {
			r.Aliases = append(r.Aliases, navdata.FeederID(a))
		}
		c.Feeders = append(c.Feeders, r)
	}
	for _, f := range p.Paths {
		r := terminal.Path{Feeder: navdata.FeederID(f.Feeder), STARFamily: navdata.STARFamilyID(f.StarFamily), FeederFix: navdata.FixID(f.FeederFix), HoldingToFeederSeconds: f.HoldingToFeederSeconds, RunwayGroup: aman.RunwayGroupID(f.RunwayGroup), MergeFix: navdata.FixID(f.MergeFix), SelectedHolding: navdata.HoldingID(f.SelectedHolding), PublishedHeadingMagneticDeg: optionalNumber[int](f.PublishedHeadingMagneticDegrees)}
		for _, a := range f.Fixes {
			r.Fixes = append(r.Fixes, navdata.FixID(a))
		}
		c.Paths = append(c.Paths, r)
	}
	for _, f := range p.StarFamilyPolicies {
		s := f.GetSameStarSpacing()
		c.STARFamilyPolicies = append(c.STARFamilyPolicies, terminal.STARFamilyPolicy{STARFamily: navdata.STARFamilyID(f.StarFamily), HoldingSequencePolicy: navdata.HoldingSequencePolicy(f.HoldingSequencePolicy), SameSTARSpacing: terminal.SameSTARSpacing{Enabled: s.GetEnabled(), ActivationRatePerHour: s.GetActivationRatePerHour(), MinimumEmptySlots: s.GetMinimumEmptySlots()}})
	}
	return c
}
