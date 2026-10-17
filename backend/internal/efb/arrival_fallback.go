package efb

import (
	"FlightStrips/internal/aman/terminal"
	"context"
	"regexp"
	"strings"
	"time"
)

var arrivalDeclarationPattern = regexp.MustCompile(`(?i)\b(?:ARR(?:IVAL|IVALS|IVING)?|LANDING|LDG)\s+(?:RWYS?|RUNWAYS?)\s+(?:IN\s+USE\s+)?([^\n.;]+)`)
var arrivalRunwayInUsePattern = regexp.MustCompile(`(?i)\b(?:RWYS?|RUNWAYS?)\s+IN\s+USE\s+([^\n.;]+)`)
var arrivalApproachPattern = regexp.MustCompile(`(?i)\bILS(?:\s+(?:APPROACH|APCH))?\s+(?:RWYS?|RUNWAYS?)?\s*((?:0[1-9]|[12][0-9]|3[0-6])[LRC]?\b[^\n.;]*)`)
var arrivalRunwayPattern = regexp.MustCompile(`\b(?:(?:04|22)[LR]|12|30)\b`)

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
	result.TerminalFix = terminalFix(a.terminal, result.Destination, result.Runway, result.STAR)
	result.PublishedHeading, result.PublishedHoldingFix, result.PublishedHoldingDetail = publishedArrivalData(
		ctx, a.navigation, a.terminal, result.Destination, result.Runway, result.STAR,
	)
}
