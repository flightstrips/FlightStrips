// Package arrivalinfo sends automatic Hoppie telex messages to arriving
// aircraft: one arrival information message about 40 minutes before the
// operational ETA, then a short message whenever the expected stand changes.
package arrivalinfo

import (
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/models"
)

// Kind identifies which automatic message was sent.
type Kind string

const (
	KindArrivalInfo Kind = "arrival_info"
	KindStandUpdate Kind = "stand_update"
)

// Delay is the slot time minus the undelayed operational ETA. Bands are
// compared on the unrounded value; the wording was agreed with operations.
func Delay(flight aman.AMANFlight) (time.Duration, bool) {
	if flight.Slot == nil || flight.Slot.Time.IsZero() || flight.Prediction == nil || flight.Prediction.OperationalTETA.IsZero() {
		return 0, false
	}
	return flight.Slot.Time.Sub(flight.Prediction.OperationalTETA), true
}

// DelayText returns the delay segment of the arrival information message.
func DelayText(delay time.Duration) string {
	switch {
	case delay < 5*time.Minute:
		return "NO DELAY EXPECTED."
	case delay <= 7*time.Minute:
		return "DELAY LESS THAN 5MIN."
	case delay <= 12*time.Minute:
		return "DELAY 5-10MIN."
	case delay <= 22*time.Minute:
		return "DELAY 10-20MIN."
	default:
		return "DELAY MORE THAN 20MIN."
	}
}

// ArrivalInfoText builds, for example,
// "EKCH ARR INFO: EXP STAND C27. MONAK5C. DELAY 5-10MIN.". The stand and STAR
// segments are left out when unknown; the delay segment always appears.
func ArrivalInfoText(station, stand, star string, delay time.Duration) string {
	var builder strings.Builder
	builder.WriteString(cleanToken(station))
	builder.WriteString(" ARR INFO:")
	if stand = cleanToken(stand); stand != "" {
		builder.WriteString(" EXP STAND ")
		builder.WriteString(stand)
		builder.WriteString(".")
	}
	if star = cleanToken(star); star != "" {
		builder.WriteString(" ")
		builder.WriteString(star)
		builder.WriteString(".")
	}
	builder.WriteString(" ")
	builder.WriteString(DelayText(delay))
	return builder.String()
}

// StandUpdateText builds, for example, "EKCH ARR INFO: NEW STAND D2.".
func StandUpdateText(station, stand string) string {
	return cleanToken(station) + " ARR INFO: NEW STAND " + cleanToken(stand) + "."
}

// ExpectedStand is the system's expected stand at any SAT stage.
func ExpectedStand(strip *models.Strip) string {
	if strip == nil || strip.Stand == nil {
		return ""
	}
	return cleanToken(*strip.Stand)
}

// ExpectedSTAR prefers the controller-assigned STAR. Without one it derives
// the full designator from AMAN's selected STAR family and the terminal
// configuration, and returns "" when that is not unambiguous.
func ExpectedSTAR(strip *models.Strip, flight aman.AMANFlight, config *terminal.Configuration) string {
	if strip != nil && strip.Star != nil {
		if star := cleanToken(*strip.Star); star != "" {
			return star
		}
	}
	if observation := flight.LatestObservation; observation != nil && observation.AssignedSTAR != nil {
		if star := cleanToken(*observation.AssignedSTAR); star != "" {
			return star
		}
	}
	return derivedSTAR(flight, config)
}

func derivedSTAR(flight aman.AMANFlight, config *terminal.Configuration) string {
	family := cleanToken(flight.STARFamilyIdentity())
	if family == "" || config == nil {
		return ""
	}
	suffixes := make(map[string]struct{})
	for _, path := range config.Paths {
		pathFamily := string(path.STARFamily)
		if pathFamily == "" {
			pathFamily = string(path.Feeder)
		}
		if cleanToken(pathFamily) != family {
			continue
		}
		if flight.SelectedRunwayGroup != nil && !sameRunwayGroup(*config, path.RunwayGroup, *flight.SelectedRunwayGroup) {
			continue
		}
		if flight.SelectedFeederFix != nil && path.FeederFix != "" && cleanToken(string(path.FeederFix)) != cleanToken(*flight.SelectedFeederFix) {
			continue
		}
		if suffix := cleanToken(path.STARSuffix); suffix != "" {
			suffixes[suffix] = struct{}{}
		}
	}
	if len(suffixes) != 1 {
		return ""
	}
	for suffix := range suffixes {
		return family + suffix
	}
	return ""
}

func sameRunwayGroup(config terminal.Configuration, pathGroup, selected aman.RunwayGroupID) bool {
	if strings.EqualFold(string(pathGroup), string(selected)) {
		return true
	}
	for _, group := range config.RunwayGroups {
		if !strings.EqualFold(string(group.ID), string(pathGroup)) {
			continue
		}
		for _, alias := range group.Aliases {
			if strings.EqualFold(string(alias), string(selected)) {
				return true
			}
		}
	}
	return false
}

// cleanToken keeps telex text to uppercase letters and digits so nothing can
// break Hoppie framing or confuse pilot clients.
func cleanToken(value string) string {
	value = strings.ToUpper(strings.TrimSpace(value))
	var builder strings.Builder
	for _, char := range value {
		if char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' {
			builder.WriteRune(char)
		}
	}
	return builder.String()
}
