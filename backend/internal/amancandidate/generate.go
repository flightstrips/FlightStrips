//go:build ignore

// Generate explicit domain/Protobuf field conversions. Run from backend with
// go run internal/amancandidate/generate.go. Unmapped fields fail generation.
package main

import (
	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	pb "FlightStrips/pkg/events/cluster"
	"fmt"
	"go/format"
	"os"
	"reflect"
	"strings"
	"time"
)

type pair struct{ domain, wire reflect.Type }

var pairs []pair

func add(a, b any)         { pairs = append(pairs, pair{reflect.TypeOf(a), reflect.TypeOf(b)}) }
func norm(s string) string { return strings.ToLower(strings.ReplaceAll(s, "_", "")) }

var aliases = map[string]string{
	"AmanFlight.GapException": "RunwayGapException", "AmanRunwayGroup.Reservations": "CapacityReservations",
	"AmanPredictionLeg.FromFix": "From", "AmanPredictionLeg.ToFix": "To",
	"NavAirport.Icao": "ID", "NavCoordinate.LatitudeDegrees": "LatitudeDeg", "NavCoordinate.LongitudeDegrees": "LongitudeDeg",
	"NavThreshold.ElevationFeet": "ElevationFt", "NavThreshold.CourseTrueDegrees": "CourseTrueDeg",
	"NavLeg.CourseTrueDegrees": "CourseTrueDeg", "NavHolding.InboundCourseTrueDegrees": "InboundCourseTrueDeg",
	"NavHolding.MinimumAltitudeFeet": "MinimumAltitudeFt", "NavHolding.MaximumAltitudeFeet": "MaximumAltitudeFt",
	"NavHolding.MaximumSpeedKnots": "MaximumSpeedKt", "NavTerminalPath.PublishedHeadingMagneticDegrees": "PublishedHeadingMagneticDeg",
}

func name(t reflect.Type) string {
	if t.PkgPath() == "" {
		return t.String()
	}
	p := t.PkgPath()[strings.LastIndex(t.PkgPath(), "/")+1:]
	if p == "cluster" {
		p = "pb"
	}
	return p + "." + t.Name()
}
func conversion(src, dst reflect.Type, x string, to bool) string {
	if src == reflect.TypeOf(time.Time{}) {
		return "timestamp(" + x + ")"
	}
	if dst == reflect.TypeOf(time.Time{}) {
		return "instant(" + x + ")"
	}
	if src == reflect.TypeOf(time.Duration(0)) {
		return "durationpb.New(" + x + ")"
	}
	if dst == reflect.TypeOf(time.Duration(0)) {
		return x + ".AsDuration()"
	}
	if src.Kind() == reflect.Pointer && src.Elem() == reflect.TypeOf(time.Time{}) {
		return "optionalTimestamp(" + x + ")"
	}
	if dst.Kind() == reflect.Pointer && dst.Elem() == reflect.TypeOf(time.Time{}) {
		return "optionalInstant(" + x + ")"
	}
	if src.Kind() == reflect.Pointer && src.Elem() == reflect.TypeOf(time.Duration(0)) {
		return "optionalDuration(" + x + ")"
	}
	if dst.Kind() == reflect.Pointer && dst.Elem() == reflect.TypeOf(time.Duration(0)) {
		return "optionalDomainDuration(" + x + ")"
	}
	se, de := src, dst
	if se.Kind() == reflect.Pointer {
		se = se.Elem()
	}
	if de.Kind() == reflect.Pointer {
		de = de.Elem()
	}
	for _, p := range pairs {
		if se == p.domain && de == p.wire || se == p.wire && de == p.domain {
			fn := "encode"
			if !to {
				fn = "decode"
			}
			arg := x
			if src.Kind() != reflect.Pointer {
				arg = "&" + x
			}
			call := fn + p.wire.Name() + "(" + arg + ")"
			if dst.Kind() != reflect.Pointer {
				call = "*" + call
			}
			return call
		}
	}
	if src.Kind() == reflect.Pointer && dst.Kind() == reflect.Pointer {
		fn := "optionalNumber"
		if de.Kind() == reflect.String {
			fn = "optionalString"
		}
		return fn + "[" + name(de) + "](" + x + ")"
	}
	if src.Kind() == reflect.Pointer || dst.Kind() == reflect.Pointer {
		panic(fmt.Sprintf("unsupported %s -> %s", src, dst))
	}
	return name(dst) + "(" + x + ")"
}
func main() {
	add(aman.AMANFlight{}, pb.AmanFlight{})
	add(aman.RunwayGroupPolicy{}, pb.AmanRunwayGroup{})
	add(aman.RunwayGroupSelectionPoint{}, pb.AmanRunwaySelectionPoint{})
	add(aman.RunwayGroupRatePoint{}, pb.AmanRunwayRatePoint{})
	add(aman.SameSTARSpacingPolicy{}, pb.AmanSameStarSpacing{})
	add(aman.RunwayGroupSequenceWarning{}, pb.AmanSequenceWarning{})
	add(aman.RunwayGap{}, pb.AmanGap{})
	add(aman.RunwayClosure{}, pb.AmanClosure{})
	add(aman.RunwayCapacityReservation{}, pb.AmanReservation{})
	add(aman.FlightObservation{}, pb.AmanFlightObservation{})
	add(aman.PlannedTiming{}, pb.AmanPlannedTiming{})
	add(aman.FlightPlanFact{}, pb.AmanFlightPlanFact{})
	add(aman.SurveillanceFact{}, pb.AmanSurveillanceFact{})
	add(aman.HoldingClearance{}, pb.AmanHoldingClearance{})
	add(aman.Prediction{}, pb.AmanPrediction{})
	add(aman.FeederETAState{}, pb.AmanFeederEta{})
	add(aman.HoldingPlan{}, pb.AmanHoldingPlan{})
	add(aman.HoldingStackState{}, pb.AmanHoldingStack{})
	add(aman.PredictionCalculation{}, pb.AmanPredictionCalculation{})
	add(aman.PredictionLeg{}, pb.AmanPredictionLeg{})
	add(aman.PredictionSegment{}, pb.AmanPredictionSegment{})
	add(aman.RawTETASample{}, pb.AmanRawTetaSample{})
	add(aman.BaselineState{}, pb.AmanBaseline{})
	add(aman.RouteProgress{}, pb.AmanRouteProgress{})
	add(aman.TMAEntryState{}, pb.AmanTmaEntry{})
	add(aman.OperationalException{}, pb.AmanOperationalException{})
	add(aman.GoAroundEvidence{}, pb.AmanGoAroundEvidence{})
	add(aman.GoAroundDetectionState{}, pb.AmanGoAroundDetection{})
	add(aman.LifecycleState{}, pb.AmanLifecycle{})
	add(aman.AbsenceState{}, pb.AmanAbsence{})
	add(aman.RouteFact{}, pb.AmanRouteFact{})
	add(aman.Slot{}, pb.AmanSlot{})
	add(aman.ETAReview{}, pb.AmanETAReview{})
	add(aman.QueueOffer{}, pb.AmanQueueOffer{})
	add(aman.GoAroundConfirmation{}, pb.AmanGoAroundConfirmation{})
	add(aman.RunwayGapException{}, pb.AmanGapException{})
	add(navdata.DatasetVersion{}, pb.NavDatasetVersion{})
	add(navdata.Provenance{}, pb.NavProvenance{})
	add(navdata.Coordinate{}, pb.NavCoordinate{})
	add(navdata.Threshold{}, pb.NavThreshold{})
	add(navdata.Airport{}, pb.NavAirport{})
	add(navdata.Runway{}, pb.NavRunway{})
	add(navdata.Procedure{}, pb.NavProcedure{})
	add(navdata.ProcedureLeg{}, pb.NavLeg{})
	add(navdata.Fix{}, pb.NavFix{})
	add(navdata.TerminalPath{}, pb.NavTerminalPath{})
	add(navdata.RouteQuery{}, pb.NavRouteQuery{})
	add(navdata.RouteGeometry{}, pb.NavRouteGeometry{})
	add(navdata.TimelineMapping{}, pb.AmanTimelineMapping{})
	add(navdata.STARFamilyPolicy{}, pb.NavStarFamilyPolicy{})
	// NavHolding's closed oneof is converted by hand in convert.go.
	add(navdata.HoldingPattern{}, pb.NavHolding{})
	out := `// Code generated by generate.go; DO NOT EDIT.
package amancandidate
import (
 "time"
 "FlightStrips/internal/aman"
 "FlightStrips/internal/aman/navdata"
 pb "FlightStrips/pkg/events/cluster"
 "google.golang.org/protobuf/types/known/durationpb"
 "google.golang.org/protobuf/types/known/timestamppb"
)
`
	for _, p := range pairs {
		if p.wire.Name() == "NavHolding" || p.wire.Name() == "NavStarFamilyPolicy" {
			continue
		}
		mapping := map[string]string{}
		seen := map[string]bool{}
		for i := 0; i < p.wire.NumField(); i++ {
			w := p.wire.Field(i)
			if !w.IsExported() {
				continue
			}
			want := aliases[p.wire.Name()+"."+w.Name]
			if want == "" {
				want = w.Name
			}
			for j := 0; j < p.domain.NumField(); j++ {
				d := p.domain.Field(j)
				if norm(d.Name) == norm(want) {
					mapping[w.Name] = d.Name
					seen[d.Name] = true
				}
			}
			if mapping[w.Name] == "" && !(p.wire.Name() == "AmanFlight" && (w.Name == "SourceObservations" || w.Name == "HoldingEatProjection")) {
				panic("unmapped wire field " + p.wire.Name() + "." + w.Name)
			}
		}
		for j := 0; j < p.domain.NumField(); j++ {
			d := p.domain.Field(j)
			if !seen[d.Name] {
				if p.domain.Name() == "TerminalPath" && d.Name == "Feeder" {
					continue
				}
				panic("unmapped domain field " + p.domain.Name() + "." + d.Name)
			}
		}
		for _, to := range []bool{true, false} {
			src, dst := p.domain, p.wire
			fn := "encode"
			if !to {
				src, dst = p.wire, p.domain
				fn = "decode"
			}
			out += fmt.Sprintf("func %s%s(v *%s) *%s { if v==nil{return nil}; r:= &%s{}\n", fn, p.wire.Name(), name(src), name(dst), name(dst))
			for i := 0; i < p.wire.NumField(); i++ {
				w := p.wire.Field(i)
				dname, ok := mapping[w.Name]
				if !ok {
					continue
				}
				d, _ := p.domain.FieldByName(dname)
				s, t := d, w
				if !to {
					s, t = w, d
				}
				if s.Type.Kind() == reflect.Slice {
					out += fmt.Sprintf("if v.%s != nil { r.%s=make(%s,len(v.%s)); for i:= range v.%s { r.%s[i]=%s } }\n", s.Name, t.Name, sliceName(t.Type), s.Name, s.Name, t.Name, conversion(s.Type.Elem(), t.Type.Elem(), "v."+s.Name+"[i]", to))
				} else {
					out += fmt.Sprintf("r.%s=%s\n", t.Name, conversion(s.Type, t.Type, "v."+s.Name, to))
				}
			}
			if !to && p.domain.Name() == "TerminalPath" {
				out += "r.Feeder=navdata.FeederID(r.STARFamily)\n"
			}
			out += "return r\n}\n"
		}
	}
	data, err := format.Source([]byte(out))
	if err != nil {
		panic(err)
	}
	if err = os.WriteFile("internal/amancandidate/convert_generated.go", data, 0644); err != nil {
		panic(err)
	}
}
func sliceName(t reflect.Type) string {
	e := t.Elem()
	if e.Kind() == reflect.Pointer {
		return "[]*" + name(e.Elem())
	}
	return "[]" + name(e)
}
