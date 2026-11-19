package efb

import (
	"FlightStrips/internal/aman/terminal"
	"context"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var arrivalDeclarationPattern = regexp.MustCompile(`(?i)\b(?:ARR(?:IVAL|IVALS|IVING)?|LANDING|LDG)\s+(?:RWYS?|RUNWAYS?)\s+(?:IN\s+USE\s+)?([^\n.;]+)`)
var arrivalRunwayInUsePattern = regexp.MustCompile(`(?i)\b(?:RWYS?|RUNWAYS?)\s+IN\s+USE\s+([^\n.;]+)`)
var arrivalApproachPattern = regexp.MustCompile(`(?i)\bILS(?:\s+(?:APPROACH|APCH))?\s+(?:RWYS?|RUNWAYS?)?\s*((?:0[1-9]|[12][0-9]|3[0-6])[LRC]?\b[^\n.;]*)`)
var arrivalRunwayPattern = regexp.MustCompile(`\b(?:(?:04|22)[LR]|12|30)\b`)

// addMETARFallback supplies the airport METAR when its ATIS is offline or stale.
func (a *WebAPI) addMETARFallback(ctx context.Context, result *snapshot, airport string) {
	if a.metar == nil || (result.ATIS != nil && !result.ATIS.Stale) {
		return
	}
	if raw := strings.TrimSpace(a.metar.GetMETAR(ctx, airport)); raw != "" {
		result.METAR = &raw
	}
}

// metarWindPattern matches the surface wind group, e.g. 24012KT, 24012G25KT, 24006MPS, VRB02KT.
var metarWindPattern = regexp.MustCompile(`\b(\d{3}|VRB)(\d{2,3})(?:G\d{2,3})?(KT|MPS)\b`)

// metarWind returns the mean wind (true direction, speed in knots). Calm and
// variable winds report ok=false: they don't favour any runway.
func metarWind(raw string) (direction int, speedKt int, ok bool) {
	match := metarWindPattern.FindStringSubmatch(strings.ToUpper(raw))
	if match == nil || match[1] == "VRB" {
		return 0, 0, false
	}
	direction, errDirection := strconv.Atoi(match[1])
	speed, errSpeed := strconv.Atoi(match[2])
	if errDirection != nil || errSpeed != nil || speed == 0 || direction > 360 {
		return 0, 0, false
	}
	if match[3] == "MPS" {
		speed = int(math.Round(float64(speed) * 1.943844))
	}
	return direction, speed, true
}

// crosswindLimitKt is the crosswind on the primary runways above which a
// secondary (crosswind) runway is preferred.
const crosswindLimitKt = 20.0

// secondaryArrivalRunways are only used when the primary runways exceed the
// crosswind limit. Airports not listed treat every runway as primary.
var secondaryArrivalRunways = map[string]map[string]bool{
	"EKCH": {"12": true, "30": true},
}

// headwindRunway picks the primary arrival runway with the largest headwind
// component. If that runway's crosswind exceeds crosswindLimitKt, the
// secondary runway with the largest headwind is used instead, provided it has
// less crosswind. On a tie (parallel runways) the first runway in
// configuration order wins, so the terminal configuration defines the
// preferred parallel.
func headwindRunway(config terminal.Configuration, direction, speedKt int) *string {
	secondary := secondaryArrivalRunways[strings.ToUpper(string(config.Airport))]
	type candidate struct {
		runway              string
		headwind, crosswind float64
	}
	var primaryBest, secondaryBest *candidate
	for _, group := range config.RunwayGroups {
		for _, approach := range group.FinalApproaches {
			angle := (float64(direction) - approach.CourseTrueDeg) * math.Pi / 180
			current := candidate{string(approach.Runway), float64(speedKt) * math.Cos(angle), math.Abs(float64(speedKt) * math.Sin(angle))}
			best := &primaryBest
			if secondary[current.runway] {
				best = &secondaryBest
			}
			if *best == nil || current.headwind > (*best).headwind+1e-6 {
				*best = &current
			}
		}
	}
	chosen := primaryBest
	if chosen == nil || (chosen.crosswind > crosswindLimitKt && secondaryBest != nil && secondaryBest.crosswind < chosen.crosswind) {
		chosen = secondaryBest
	}
	if chosen == nil {
		return nil
	}
	return &chosen.runway
}

func arrivalRunwayFromATIS(text string) *string {
	text = strings.ToUpper(text)
	for _, pattern := range []*regexp.Regexp{arrivalDeclarationPattern, arrivalRunwayInUsePattern, arrivalApproachPattern} {
		runways := make(map[string]bool)
		for _, declaration := range pattern.FindAllStringSubmatchIndex(text, -1) {
			if pattern == arrivalRunwayInUsePattern {
				prefix := text[:declaration[0]]
				if index := strings.LastIndexAny(prefix, "\n.;"); index >= 0 {
					prefix = prefix[index+1:]
				}
				if strings.Contains(prefix, "DEP") || strings.Contains(prefix, "TAKE-OFF") {
					continue
				}
			}
			clause := text[declaration[2]:declaration[3]]
			for _, boundary := range []string{"WIND", "QNH", "TEMPERATURE", "DEPARTURE", "TAKE-OFF", "TRANSITION"} {
				if index := strings.Index(clause, boundary); index >= 0 {
					clause = clause[:index]
				}
			}
			for _, runway := range arrivalRunwayPattern.FindAllString(clause, -1) {
				runways[runway] = true
			}
		}
		if len(runways) > 1 {
			return nil
		}
		for runway := range runways {
			return &runway
		}
	}
	return nil
}

func filedArrivalFeeder(config terminal.Configuration, route, destination string) *string {
	tokens := strings.Fields(strings.ToUpper(route))
	for index := len(tokens) - 1; index >= 0; index-- {
		token := strings.SplitN(tokens[index], "/", 2)[0]
		if token == "DCT" || token == strings.ToUpper(destination) || filedSpeedLevelPattern.MatchString(token) {
			continue
		}
		for _, feeder := range config.Feeders {
			if token == string(feeder.ID) {
				return &token
			}
			for _, alias := range feeder.Aliases {
				if token == string(alias) {
					value := string(feeder.ID)
					return &value
				}
			}
		}
		return nil
	}
	return nil
}

func (a *WebAPI) addArrivalGuidance(ctx context.Context, result *snapshot) {
	if result.Phase != "ARRIVAL" || !strings.EqualFold(result.Destination, string(a.terminal.Airport)) {
		return
	}
	now := time.Now()
	if now.Before(a.terminal.ApplicabilityFrom) || !now.Before(a.terminal.ApplicabilityUntil) {
		result.DataErrors = append(result.DataErrors, "Published arrival guidance is unavailable outside the configured AIRAC validity period.")
		return
	}
	if result.STAR == nil && result.Route != nil {
		result.STAR = filedArrivalFeeder(a.terminal, *result.Route, result.Destination)
		result.ArrivalGuidanceExpected = result.STAR != nil
	}
	if result.Runway == nil && result.ATIS != nil && !result.ATIS.Stale {
		result.Runway = arrivalRunwayFromATIS(strings.Join(result.ATIS.Text, "\n"))
		result.ArrivalGuidanceExpected = result.ArrivalGuidanceExpected || result.Runway != nil
	}
	if result.Runway == nil && a.metar != nil {
		if direction, speed, ok := metarWind(a.metar.GetMETAR(ctx, result.Destination)); ok {
			result.Runway = headwindRunway(a.terminal, direction, speed)
			result.ArrivalGuidanceExpected = result.ArrivalGuidanceExpected || result.Runway != nil
		}
	}
	result.TerminalFix = terminalFix(a.terminal, result.Destination, result.Runway, result.STAR)
	result.PublishedHeading, result.PublishedHoldingFix, result.PublishedHoldingDetail = publishedArrivalData(
		ctx, a.navigation, a.terminal, result.Destination, result.Runway, result.STAR,
	)
}
