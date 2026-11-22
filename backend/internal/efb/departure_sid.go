package efb

import (
	"regexp"
	"strings"
)

// ekchSIDsByRunway lists the published EKCH SIDs per departure runway
// (AIRAC 2611, VATSCA EKDK sector file). The letter identifies the runway and
// the number is per procedure, so a filed SID name plus the runway in use
// identifies exactly one procedure.
var ekchSIDsByRunway = map[string][]string{
	"04L": {"BETUD2E", "GOLGA3E", "KEMAX2E", "KOPEX2E", "LANGO2E", "NEXEN2E", "ODDON1E", "SALLO1E", "SIMEG2E", "VEDAR3E"},
	"04R": {"BETUD2A", "GOLGA3A", "KEMAX5A", "KOPEX2A", "LANGO2A", "NEXEN2A", "ODDON1A", "SALLO1A", "SIMEG8A", "VEDAR4A"},
	"12":  {"BETUD3B", "GOLGA3B", "KEMAX6B", "KOPEX3B", "LANGO3B", "NEXEN3B", "ODDON1B", "SALLO1B", "SIMEG9B", "VEDAR3B"},
	"22L": {"BETUD2F", "GOLGA2F", "KEMAX2F", "KOPEX2F", "LANGO2F", "NEXEN2F", "ODDON1F", "SALLO1F", "SIMEG2F", "VEDAR2F"},
	"22R": {"BETUD2C", "GOLGA2C", "KEMAX5C", "KOPEX2C", "LANGO2C", "NEXEN2C", "ODDON1C", "SALLO1C", "SIMEG9C", "VEDAR2C"},
	"30":  {"BETUD3D", "GOLGA3D", "KEMAX6D", "KOPEX3D", "LANGO3D", "NEXEN3D", "ODDON1D", "SALLO1D", "SIMEG9D", "VEDAR3D"},
}

// The sector file abbreviates two SID names; flight plans may use either form.
var ekchSIDNameAliases = map[string]string{"ODON": "ODDON", "SALO": "SALLO"}

var bareSIDNamePattern = regexp.MustCompile(`^[A-Z]{4,5}$`)

// withPublishedSIDVersion turns a bare SID name taken from the filed route
// (e.g. NEXEN) into the published procedure for the runway in use (e.g.
// NEXEN2C). A SID that already carries a version, such as a controller's
// assignment, is returned unchanged, as is one that cannot be resolved.
func withPublishedSIDVersion(origin string, runway, sid *string) *string {
	if sid == nil || runway == nil || !strings.EqualFold(strings.TrimSpace(origin), "EKCH") {
		return sid
	}
	name := strings.ToUpper(strings.TrimSpace(*sid))
	if !bareSIDNamePattern.MatchString(name) {
		return sid
	}
	if full, ok := ekchSIDNameAliases[name]; ok {
		name = full
	}
	for _, procedure := range ekchSIDsByRunway[strings.ToUpper(strings.TrimSpace(*runway))] {
		if strings.HasPrefix(procedure, name) && len(procedure) == len(name)+2 {
			return &procedure
		}
	}
	return sid
}
