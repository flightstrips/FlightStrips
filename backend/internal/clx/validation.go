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
	rules := ctx.Rules
	if rules.IsEmpty() {
		return nil
	}

	var faults []Fault
	faults = append(faults, sidEngineFaults(strip, rules)...)
	faults = append(faults, aircraftRunwayFaults(strip, rules)...)
	if fault, ok := rnavFault(strip, rules); ok {
		faults = append(faults, fault)
	}
	for _, fault := range routeConflictFaults(strip, rules) {
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

func sidEngineFaults(strip *models.Strip, rules cfgpkg.ClxValidationConfig) []Fault {
	family := sidFamily(value(strip.Sid))
	engine := strings.ToUpper(strings.TrimSpace(strip.EngineType))
	if family == "" || engine == "" {
		return nil
	}

	faults := make([]Fault, 0, len(rules.SidEngineRules))
	for _, rule := range rules.SidEngineRules {
		if !slices.Contains(rule.SidFamilies, family) || !slices.Contains(rule.DisallowedEngineTypes, engine) {
			continue
		}

		message := renderMessage(rule.Message, map[string]string{
			"sid_family":     family,
			"engine_type":    engine,
			"recommendation": sidEngineRecommendation(strip, rule),
		})
		if strings.TrimSpace(rule.Code) == "" || strings.TrimSpace(message) == "" {
			continue
		}

		faults = append(faults, newFault(rule.Code, message, []string{FieldSID}, ""))
	}

	return faults
}

func sidEngineRecommendation(strip *models.Strip, rule cfgpkg.ClxSidEngineRule) string {
	for _, token := range tokens(combinedFlightPlanText(strip)) {
		if recommendation, ok := rule.Recommendations[token]; ok {
			return recommendation
		}
		trimmedToken := strings.TrimSuffix(token, "/")
		if recommendation, ok := rule.Recommendations[trimmedToken]; ok {
			return recommendation
		}
	}
	return strings.TrimSpace(rule.DefaultRecommendation)
}

func aircraftRunwayFaults(strip *models.Strip, rules cfgpkg.ClxValidationConfig) []Fault {
	aircraft := normalizedAircraftType(value(strip.AircraftType))
	runway := strings.ToUpper(strings.TrimSpace(value(strip.Runway)))

	faults := make([]Fault, 0, len(rules.AircraftRunwayRules))
	for _, rule := range rules.AircraftRunwayRules {
		if !slices.Contains(rule.AircraftTypes, aircraft) {
			continue
		}

		var fields []string
		if slices.Contains(rule.RestrictedRunways, runway) {
			fields = append(fields, FieldRunway)
		}
		if sidHasRestrictedSuffix(value(strip.Sid), rule.RestrictedSidSuffixes) {
			fields = append(fields, FieldSID)
		}
		if len(fields) == 0 {
			continue
		}

		message := renderMessage(rule.Message, map[string]string{
			"aircraft_type": aircraft,
		})
		if strings.TrimSpace(rule.Code) == "" || strings.TrimSpace(message) == "" {
			continue
		}

		faults = append(faults, newFault(rule.Code, message, fields, ""))
	}

	return faults
}

func rnavFault(strip *models.Strip, rules cfgpkg.ClxValidationConfig) (Fault, bool) {
	if !sidFilingDetected(strip, rules) {
		return Fault{}, false
	}
	if headingVectorWithinRnavLimit(strip) {
		return Fault{}, false
	}

	capability := rnav.DeriveCapability(value(strip.AircraftType), value(strip.Remarks))
	switch capability {
	case "NIL":
		if rules.RnavRules.Nil.Code == "" || rules.RnavRules.Nil.Message == "" {
			return Fault{}, false
		}
		return newFault(rules.RnavRules.Nil.Code, rules.RnavRules.Nil.Message, []string{FieldRNAV}, ""), true
	default:
		if !slices.Contains(rules.RnavRules.Insufficient.Capabilities, capability) {
			return Fault{}, false
		}
		if rules.RnavRules.Insufficient.Code == "" || rules.RnavRules.Insufficient.Message == "" {
			return Fault{}, false
		}
		return newFault(rules.RnavRules.Insufficient.Code, rules.RnavRules.Insufficient.Message, []string{FieldRNAV}, ""), true
	}
}

func headingVectorWithinRnavLimit(strip *models.Strip) bool {
	heading := value(strip.Heading)
	requestedAltitude := value(strip.RequestedAltitude)
	return heading > 0 && requestedAltitude > 0 && requestedAltitude <= 28000
}

func routeConflictFaults(strip *models.Strip, rules cfgpkg.ClxValidationConfig) []Fault {
	family := sidFamily(value(strip.Sid))
	if family == "" {
		return nil
	}

	text := combinedFlightPlanText(strip)

	faults := make([]Fault, 0, len(rules.RouteConflictRules))
	for _, rule := range rules.RouteConflictRules {
		if !slices.Contains(rule.SidFamilies, family) {
			continue
		}
		if !rule.Always && !containsAnyToken(text, rule.RouteTokensAny) && !containsAnyToken(text, rule.RemarkTokensAny) {
			continue
		}
		if strings.TrimSpace(rule.Code) == "" || strings.TrimSpace(rule.Message) == "" {
			continue
		}

		overrideKey := ""
		if rule.AllowOverride {
			overrideKey = routeOverrideKey(strip, rule.Code)
		}
		faults = append(faults, newFault(rule.Code, rule.Message, []string{FieldSID}, overrideKey))
	}

	return faults
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
		Message:     "TOBT and EOBT are in the past. Click TOBT to Update, or manually enter EOBT.",
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

func sidHasRestrictedSuffix(sid string, restrictedSuffixes []string) bool {
	sid = strings.ToUpper(strings.TrimSpace(sid))
	if sid == "" {
		return false
	}
	return slices.Contains(restrictedSuffixes, sid[len(sid)-1:])
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

func newFault(code string, message string, fields []string, overrideKey string) Fault {
	return Fault{
		Code:        code,
		Message:     message,
		NitosRemark: message,
		Fields:      fields,
		OverrideKey: overrideKey,
	}
}

func renderMessage(template string, replacements map[string]string) string {
	rendered := strings.TrimSpace(template)
	for key, value := range replacements {
		rendered = strings.ReplaceAll(rendered, "{"+key+"}", value)
	}
	return rendered
}

func value[T any](ptr *T) T {
	var zero T
	if ptr == nil {
		return zero
	}
	return *ptr
}
