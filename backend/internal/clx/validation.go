package clx

import (
	cfgpkg "FlightStrips/internal/config"
	"FlightStrips/internal/models"
	"FlightStrips/internal/rnav"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
)

const (
	FieldSID    = "sid"
	FieldRunway = "runway"
	FieldRNAV   = "rnav"
	FieldEOBT   = "eobt"
	FieldTOBT   = "tobt"
)

type Validation struct {
	Faults []Fault
}

type Fault struct {
	Code        string
	Message     string
	NitosRemark string
	Fields      []string
	OverrideKey string
}

type Context struct {
	Now       time.Time
	Overrides map[string]bool
	Rules     cfgpkg.ClxValidationConfig
}

var routeTokenSplitter = regexp.MustCompile(`[^A-Z0-9/]+`)

func Validate(strip *models.Strip, ctx Context) *Validation {
	if strip == nil {
		return nil
	}
	if ctx.Now.IsZero() {
		ctx.Now = time.Now().UTC()
	}
	rules := validationRules(ctx.Rules)

	var faults []Fault
	faults = append(faults, sidTypeFaults(strip, rules)...)
	faults = append(faults, runwayCategoryFaults(strip, rules)...)
	if fault, ok := rnavFault(strip, rules); ok {
		faults = append(faults, fault)
	}
	for _, fault := range routeSidFaults(strip, rules) {
		if fault.OverrideKey != "" && ctx.Overrides[fault.OverrideKey] {
			continue
		}
		faults = append(faults, fault)
	}
	if fault, ok := pastEobtTobtFault(strip, ctx.Now); ok {
		faults = append(faults, fault)
	}

	if len(faults) == 0 {
		return nil
	}
	return &Validation{Faults: faults}
}

func validationRules(rules cfgpkg.ClxValidationConfig) cfgpkg.ClxValidationConfig {
	if len(rules.JetRestrictedSidFamilies) == 0 &&
		len(rules.PropTurbopropRestrictedSidFamilies) == 0 &&
		len(rules.CategoryFAircraftTypes) == 0 &&
		len(rules.CategoryFRestrictedRunways) == 0 &&
		len(rules.CategoryFRestrictedSidSuffixes) == 0 &&
		len(rules.SidFirstWaypoints) == 0 &&
		len(rules.LangoRouteTokens) == 0 &&
		len(rules.LangoRemarkTokens) == 0 &&
		len(rules.VedarRouteTokens) == 0 &&
		len(rules.VedarRemarkTokens) == 0 {
		return cfgpkg.DefaultClxValidationConfig()
	}
	return rules
}

func sidTypeFaults(strip *models.Strip, rules cfgpkg.ClxValidationConfig) []Fault {
	family := sidFamily(value(strip.Sid))
	engine := strings.ToUpper(strings.TrimSpace(strip.EngineType))
	if family == "" || engine == "" {
		return nil
	}

	switch {
	case slices.Contains(rules.JetRestrictedSidFamilies, family) && engine == "J":
		return []Fault{{
			Code:        "sid_aircraft_type",
			Message:     fmt.Sprintf("SID %s is not valid for aircraft engine type J.", family),
			NitosRemark: "Aircraft planned on SID not valid for ATYP. " + sidTypeRecommendation(strip),
			Fields:      []string{FieldSID},
		}}
	case slices.Contains(rules.PropTurbopropRestrictedSidFamilies, family) && (engine == "P" || engine == "T"):
		return []Fault{{
			Code:        "sid_aircraft_type",
			Message:     fmt.Sprintf("SID %s is not valid for aircraft engine type %s.", family, engine),
			NitosRemark: "Aircraft planned on SID not valid for ATYP. " + sidTypeRecommendation(strip),
			Fields:      []string{FieldSID},
		}}
	default:
		return nil
	}
}

func sidTypeRecommendation(strip *models.Strip) string {
	text := combinedFlightPlanText(strip)
	switch {
	case containsToken(text, "MICOS"):
		return "Reclear on NEXEN T503 MICOS... as filed"
	case containsToken(text, "ALS"):
		return "Reclear on LANGO P999 AMRAK... as filed"
	case containsToken(text, "ALASA"):
		return "Reclear on LANGO M611 ALASA... as filed"
	default:
		return "Reclear on LANGO DCT... as filed"
	}
}

func runwayCategoryFaults(strip *models.Strip, rules cfgpkg.ClxValidationConfig) []Fault {
	aircraft := normalizedAircraftType(value(strip.AircraftType))
	if !slices.Contains(rules.CategoryFAircraftTypes, aircraft) {
		return nil
	}

	var fields []string
	runway := strings.ToUpper(strings.TrimSpace(value(strip.Runway)))
	if slices.Contains(rules.CategoryFRestrictedRunways, runway) {
		fields = append(fields, FieldRunway)
	}
	if sidHasInvalidCategoryFSuffix(value(strip.Sid), rules) {
		fields = append(fields, FieldSID)
	}
	if len(fields) == 0 {
		return nil
	}

	return []Fault{{
		Code:        "category_f_runway",
		Message:     fmt.Sprintf("Aircraft type %s is restricted to 04R/22L.", aircraft),
		NitosRemark: "Planned RWY not available for aircraft Category (CAT F). Only 04R/22L approved",
		Fields:      fields,
	}}
}

func rnavFault(strip *models.Strip, rules cfgpkg.ClxValidationConfig) (Fault, bool) {
	if !sidFilingDetected(strip, rules) {
		return Fault{}, false
	}

	capability := rnav.DeriveCapability(value(strip.AircraftType), value(strip.Remarks))
	switch capability {
	case "NIL":
		return Fault{
			Code:        "rnav_nil",
			Message:     "Aircraft filed on SID without RNAV capability.",
			NitosRemark: "Aircraft filed on SID without RNAV capability. Clear via RV or update RNAV capability to \"1\".",
			Fields:      []string{FieldRNAV},
		}, true
	case "5", "10":
		return Fault{
			Code:        "rnav_insufficient",
			Message:     "Aircraft filed on SID with insufficient RNAV capability.",
			NitosRemark: "Aircraft filed on SID with insufficient RNAV capability. Clear via RV or update RNAV capability to \"1\".",
			Fields:      []string{FieldRNAV},
		}, true
	default:
		return Fault{}, false
	}
}

func routeSidFaults(strip *models.Strip, rules cfgpkg.ClxValidationConfig) []Fault {
	family := sidFamily(value(strip.Sid))
	if family == "" {
		return nil
	}

	text := combinedFlightPlanText(strip)
	switch family {
	case "LANGO":
		if containsAnyToken(text, rules.LangoRemarkTokens) || containsAnyToken(text, rules.LangoRouteTokens) {
			return []Fault{routeFault(strip, "route_lango_egpx", "LANGO not valid for flights to EGPX. Refile to ODDON.")}
		}
	case "VEDAR":
		if containsAnyToken(text, rules.VedarRemarkTokens) || containsAnyToken(text, rules.VedarRouteTokens) {
			return []Fault{routeFault(strip, "route_vedar_ekdk", "VEDAR not valid for re-entering EKDK. Refile to GOLGA")}
		}
	case "BETUD":
		return []Fault{routeFault(strip, "route_betud", "BETUD not valid for flight. Refile via SALLO (Or SIMEG if more appropriate).")}
	case "SIMEG":
		if containsToken(text, "SALLO") {
			return []Fault{routeFault(strip, "route_simeg_sallo", "SIMEG not valid as SID, when SALLO is on FPL. Refile to SALLO")}
		}
	}
	return nil
}

func routeFault(strip *models.Strip, code string, remark string) Fault {
	return Fault{
		Code:        code,
		Message:     remark,
		NitosRemark: remark,
		Fields:      []string{FieldSID},
		OverrideKey: routeOverrideKey(strip, code),
	}
}

func routeOverrideKey(strip *models.Strip, code string) string {
	return strings.Join([]string{
		code,
		strings.ToUpper(strings.TrimSpace(strip.Callsign)),
		strings.ToUpper(strings.TrimSpace(value(strip.Sid))),
		strings.ToUpper(strings.TrimSpace(value(strip.Route))),
		strings.ToUpper(strings.TrimSpace(value(strip.Remarks))),
	}, "|")
}

func pastEobtTobtFault(strip *models.Strip, now time.Time) (Fault, bool) {
	eobt := value(strip.EffectiveEobt())
	tobt := value(strip.EffectiveTobt())
	if eobt == "" || tobt == "" {
		return Fault{}, false
	}
	if !clockValueInPast(eobt, now) || !clockValueInPast(tobt, now) {
		return Fault{}, false
	}
	return Fault{
		Code:        "eobt_tobt_past",
		Message:     "TOBT and EOBT are in the past.",
		NitosRemark: "TOBT and EOBT are in the past. Click TOBT to Update, or manually enter EOBT.",
		Fields:      []string{FieldEOBT, FieldTOBT},
	}, true
}

func clockValueInPast(raw string, now time.Time) bool {
	raw = strings.TrimSpace(raw)
	if len(raw) < 4 {
		return false
	}
	var hour, minute int
	if _, err := fmt.Sscanf(raw[:4], "%2d%2d", &hour, &minute); err != nil {
		return false
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return false
	}

	now = now.UTC()
	candidate := time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, time.UTC)
	if candidate.Sub(now) > 12*time.Hour {
		candidate = candidate.Add(-24 * time.Hour)
	} else if now.Sub(candidate) > 12*time.Hour {
		candidate = candidate.Add(24 * time.Hour)
	}
	return candidate.Before(now)
}

func sidFilingDetected(strip *models.Strip, rules cfgpkg.ClxValidationConfig) bool {
	if sidFamily(value(strip.Sid)) != "" {
		return true
	}
	first := firstRouteToken(value(strip.Route))
	if first == "" {
		return false
	}
	return slices.Contains(rules.SidFirstWaypoints, first)
}

func sidFamily(sid string) string {
	sid = strings.ToUpper(strings.TrimSpace(sid))
	if sid == "" {
		return ""
	}
	for i, r := range sid {
		if r < 'A' || r > 'Z' {
			if i == 0 {
				return ""
			}
			return sid[:i]
		}
	}
	return sid
}

func sidHasInvalidCategoryFSuffix(sid string, rules cfgpkg.ClxValidationConfig) bool {
	sid = strings.ToUpper(strings.TrimSpace(sid))
	if sid == "" {
		return false
	}
	return slices.Contains(rules.CategoryFRestrictedSidSuffixes, sid[len(sid)-1:])
}

func normalizedAircraftType(aircraftType string) string {
	aircraftType = strings.ToUpper(strings.TrimSpace(aircraftType))
	if beforeSlash, _, ok := strings.Cut(aircraftType, "/"); ok {
		return beforeSlash
	}
	return aircraftType
}

func combinedFlightPlanText(strip *models.Strip) string {
	return strings.ToUpper(value(strip.Route) + " " + value(strip.Remarks))
}

func firstRouteToken(route string) string {
	for _, token := range tokens(route) {
		if strings.HasSuffix(token, "/") {
			continue
		}
		return strings.TrimSuffix(token, "/")
	}
	return ""
}

func containsAnyToken(text string, values []string) bool {
	for _, value := range values {
		if containsToken(text, value) {
			return true
		}
	}
	return false
}

func containsToken(text string, token string) bool {
	token = strings.ToUpper(strings.TrimSpace(token))
	trimmedToken := strings.TrimSuffix(token, "/")
	for _, candidate := range tokens(text) {
		trimmedCandidate := strings.TrimSuffix(candidate, "/")
		if candidate == token || trimmedCandidate == trimmedToken {
			return true
		}
		if strings.Contains(token, "/") && strings.HasSuffix(candidate, token) {
			return true
		}
	}
	return false
}

func tokens(text string) []string {
	text = strings.ToUpper(text)
	fields := routeTokenSplitter.Split(text, -1)
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			result = append(result, field)
		}
	}
	return result
}

func value[T any](ptr *T) T {
	var zero T
	if ptr == nil {
		return zero
	}
	return *ptr
}
