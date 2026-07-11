package sat

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
)

// BorderStatus is the border classification used by airline assignment
// conditions. An empty value means that the rule is not border-specific.
type BorderStatus string

const (
	BorderStatusSchengen    BorderStatus = "SCHENGEN"
	BorderStatusNonSchengen BorderStatus = "NON_SCHENGEN"
)

// AssignmentDirection identifies the flight leg to which a rule applies.
type AssignmentDirection string

const (
	AssignmentDirectionArrival   AssignmentDirection = "ARRIVAL"
	AssignmentDirectionDeparture AssignmentDirection = "DEPARTURE"
)

// AirlineAssignmentConditions is deliberately structured so configuration
// cannot smuggle GRPlugin's free-form preference expressions into SAT.
type AirlineAssignmentConditions struct {
	BorderStatus  BorderStatus        `json:"border_status,omitempty"`
	AircraftTypes []string            `json:"aircraft_types,omitempty"`
	AircraftUse   []AircraftUseCode   `json:"aircraft_use,omitempty"`
	Direction     AssignmentDirection `json:"direction,omitempty"`
	Special       string              `json:"special,omitempty"`
}

// WeightedStand is one weighted physical stand or one weighted named group.
// Exactly one of Stand and Group must be set.
type WeightedStand struct {
	Stand  string  `json:"stand,omitempty"`
	Group  string  `json:"group,omitempty"`
	Weight float64 `json:"weight"`
}

// StandTier is evaluated in list order. Entries with no compatible positive
// weight are skipped by the later selection task.
type StandTier struct {
	Name    string          `json:"name,omitempty"`
	Entries []WeightedStand `json:"entries"`
}

// AirlineAssignmentRule contains airline/callsign-specific preferences.
type AirlineAssignmentRule struct {
	ID         string                        `json:"id,omitempty"`
	Callsigns  []string                      `json:"callsigns"`
	Conditions *AirlineAssignmentConditions  `json:"conditions,omitempty"`
	Stands     map[string]map[string]float64 `json:"stands"`

	// Tiers is the validated, ordered representation used by later selection
	// code. It is deliberately not part of the JSON contract.
	Tiers []StandTier `json:"-"`
}

// FallbackRule is a named preference used when no airline rule applies or an
// airline rule has no eligible tier.
type FallbackRule struct {
	Stands map[string]map[string]float64 `json:"stands"`
	Tiers  []StandTier                   `json:"-"`
}

const (
	FallbackAirlinerDefault    = "airliner_default"
	FallbackBusinessVIP        = "business_vip"
	FallbackCargo              = "cargo"
	FallbackMilitary           = "military"
	FallbackMilitaryHelicopter = "military_helicopter"
	FallbackHelicopter         = "helicopter"
	FallbackGAPrivate          = "ga_private"
	FallbackUnknown            = "unknown"
)

var requiredFallbacks = []string{
	FallbackAirlinerDefault,
	FallbackBusinessVIP,
	FallbackCargo,
	FallbackMilitary,
	FallbackMilitaryHelicopter,
	FallbackHelicopter,
	FallbackGAPrivate,
	FallbackUnknown,
}

// AirlineAssignmentConfig is the complete SAT preference configuration.
// StandGroups are expanded before a tier is considered by the selector.
type AirlineAssignmentConfig struct {
	Rules         []AirlineAssignmentRule `json:"rules"`
	StandGroups   map[string][]string     `json:"stand_groups"`
	FallbackRules map[string]FallbackRule `json:"fallback_rules"`

	standRegistry *StandCapabilityRegistry
}

// AssignmentFlightFacts contains only the facts needed to choose a rule. The
// compatibility task is responsible for resolving these facts first.
type AssignmentFlightFacts struct {
	Callsign     string
	AircraftType string
	AircraftUse  AircraftUseCode
	BorderStatus BorderStatus
	Direction    AssignmentDirection
	Special      string
}

// RulePrecedence identifies which matching tier selected a rule.
type RulePrecedence string

const (
	RulePrecedenceSpecial         RulePrecedence = "special"
	RulePrecedenceExactCallsign   RulePrecedence = "exact_callsign"
	RulePrecedenceWildcard        RulePrecedence = "wildcard"
	RulePrecedenceAirlinePrefix   RulePrecedence = "airline_prefix"
	RulePrecedenceUseFallback     RulePrecedence = "use_fallback"
	RulePrecedenceDefaultFallback RulePrecedence = "default_fallback"
)

// RuleMatch describes the policy decision without selecting a physical stand.
type RuleMatch struct {
	Rule       *AirlineAssignmentRule
	Fallback   string
	Precedence RulePrecedence
	Pattern    string
}

// LoadAirlineAssignment strictly decodes and validates an airline assignment
// document against the physical stand registry. SAT must not become ready
// without this registry because physical stand references are part of the
// configuration contract.
func LoadAirlineAssignment(source io.Reader, registry *StandCapabilityRegistry) (*AirlineAssignmentConfig, error) {
	if source == nil {
		return nil, errors.New("airline assignment source is nil")
	}
	if registry == nil {
		return nil, errors.New("airline assignment stand registry is nil")
	}

	var config AirlineAssignmentConfig
	decoder := json.NewDecoder(source)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("decode airline assignment: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("decode airline assignment: multiple JSON values")
		}
		return nil, fmt.Errorf("decode airline assignment trailing data: %w", err)
	}

	config.normalize()
	if err := config.Validate(registry); err != nil {
		return nil, err
	}
	config.standRegistry = registry
	return &config, nil
}

// LoadAirlineAssignmentFile loads a committed SAT assignment document.
func LoadAirlineAssignmentFile(path string, registry *StandCapabilityRegistry) (*AirlineAssignmentConfig, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open airline assignment file %q: %w", path, err)
	}
	defer file.Close()

	config, err := LoadAirlineAssignment(file, registry)
	if err != nil {
		return nil, fmt.Errorf("load airline assignment file %q: %w", path, err)
	}
	return config, nil
}

// Validate checks schema, policy, weights, and (when provided) physical stand
// references. It does not require weights to sum to any particular total.
func (c *AirlineAssignmentConfig) Validate(registry *StandCapabilityRegistry) error {
	if c == nil {
		return errors.New("airline assignment configuration is nil")
	}
	if registry == nil {
		return errors.New("airline assignment stand registry is nil")
	}
	if c.StandGroups == nil {
		return errors.New("stand_groups must be present")
	}
	if c.FallbackRules == nil {
		return errors.New("fallback_rules must be present")
	}
	for _, name := range requiredFallbacks {
		if _, ok := lookupFallback(c.FallbackRules, name); !ok {
			return fmt.Errorf("fallback_rules is missing required %q fallback", name)
		}
	}

	groupKeys := make(map[string]string, len(c.StandGroups))
	for name, members := range c.StandGroups {
		key := normalizeGroupName(name)
		if key == "" {
			return errors.New("stand_groups contains an empty name")
		}
		if _, exists := groupKeys[key]; exists {
			return fmt.Errorf("stand_groups contains duplicate name %q", name)
		}
		groupKeys[key] = name
		if len(members) == 0 {
			return fmt.Errorf("stand group %q is empty", name)
		}
	}

	for i := range c.Rules {
		if err := validateRule(&c.Rules[i], fmt.Sprintf("rules[%d]", i), registry, groupKeys); err != nil {
			return err
		}
	}
	for name, fallback := range c.FallbackRules {
		if normalizeFallbackName(name) == "" {
			return errors.New("fallback_rules contains an empty name")
		}
		if err := validateTiers(fallback.Tiers, fmt.Sprintf("fallback_rules[%q]", name), registry, groupKeys); err != nil {
			return err
		}
	}
	for name := range c.StandGroups {
		if _, err := c.ResolveStandGroup(name, registry); err != nil {
			return err
		}
	}
	return nil
}

func validateRule(rule *AirlineAssignmentRule, path string, registry *StandCapabilityRegistry, groupKeys map[string]string) error {
	if rule.ID == "" {
		rule.ID = fmt.Sprintf("%s", path)
	}
	if len(rule.Callsigns) == 0 && (rule.Conditions == nil || rule.Conditions.Special == "") {
		return fmt.Errorf("%s.callsigns must not be empty unless conditions.special is set", path)
	}
	if rule.Stands == nil {
		return fmt.Errorf("%s.stands must be present", path)
	}
	seen := make(map[string]struct{}, len(rule.Callsigns))
	for i, callsign := range rule.Callsigns {
		if callsign == "" {
			return fmt.Errorf("%s.callsigns[%d] must not be empty", path, i)
		}
		if strings.Count(callsign, "*") > 1 || (strings.Contains(callsign, "*") && !strings.HasSuffix(callsign, "*")) {
			return fmt.Errorf("%s.callsigns[%d] must use at most one trailing wildcard", path, i)
		}
		if callsign == "*" {
			return fmt.Errorf("%s.callsigns[%d] must contain a prefix before the wildcard", path, i)
		}
		if _, exists := seen[callsign]; exists {
			return fmt.Errorf("%s contains duplicate callsign pattern %q", path, callsign)
		}
		seen[callsign] = struct{}{}
	}
	if err := validateConditions(rule.Conditions, path+".conditions"); err != nil {
		return err
	}
	if len(rule.Tiers) == 0 {
		return nil
	}
	return validateTiers(rule.Tiers, path, registry, groupKeys)
}

func validateConditions(conditions *AirlineAssignmentConditions, path string) error {
	if conditions == nil {
		return nil
	}
	if conditions.BorderStatus != "" && conditions.BorderStatus != BorderStatusSchengen && conditions.BorderStatus != BorderStatusNonSchengen {
		return fmt.Errorf("%s.border_status has unsupported value %q", path, conditions.BorderStatus)
	}
	if conditions.Direction != "" && conditions.Direction != AssignmentDirectionArrival && conditions.Direction != AssignmentDirectionDeparture {
		return fmt.Errorf("%s.direction has unsupported value %q", path, conditions.Direction)
	}
	for i, aircraftType := range conditions.AircraftTypes {
		if aircraftType == "" {
			return fmt.Errorf("%s.aircraft_types[%d] must not be empty", path, i)
		}
		if strings.Count(aircraftType, "*") > 1 || (strings.Contains(aircraftType, "*") && !strings.HasSuffix(aircraftType, "*")) {
			return fmt.Errorf("%s.aircraft_types[%d] must use at most one trailing wildcard", path, i)
		}
	}
	for i, use := range conditions.AircraftUse {
		if _, ok := validAircraftUseCodes[use]; !ok {
			return fmt.Errorf("%s.aircraft_use[%d] has unsupported value %q", path, i, use)
		}
	}
	return nil
}

func validateTiers(tiers []StandTier, path string, registry *StandCapabilityRegistry, groupKeys map[string]string) error {
	if len(tiers) == 0 {
		return fmt.Errorf("%s.tiers must not be empty", path)
	}
	if len(tiers) > 3 {
		return fmt.Errorf("%s.tiers must contain at most three tiers", path)
	}
	for tierIndex := range tiers {
		tier := &tiers[tierIndex]
		if len(tier.Entries) == 0 {
			return fmt.Errorf("%s.tiers[%d] must not be empty", path, tierIndex)
		}
		for entryIndex := range tier.Entries {
			entry := &tier.Entries[entryIndex]
			if (entry.Stand == "") == (entry.Group == "") {
				return fmt.Errorf("%s.tiers[%d].entries[%d] must specify exactly one of stand or group", path, tierIndex, entryIndex)
			}
			if math.IsNaN(entry.Weight) || math.IsInf(entry.Weight, 0) || entry.Weight < 0 {
				return fmt.Errorf("%s.tiers[%d].entries[%d].weight must be finite and non-negative", path, tierIndex, entryIndex)
			}
			if entry.Group != "" {
				if _, ok := groupKeys[normalizeGroupName(entry.Group)]; !ok {
					return fmt.Errorf("%s.tiers[%d].entries[%d] references unknown stand group %q", path, tierIndex, entryIndex, entry.Group)
				}
			}
			if registry != nil && entry.Stand != "" {
				if _, ok := registry.Lookup("EKCH", entry.Stand); !ok {
					return fmt.Errorf("%s.tiers[%d].entries[%d] references unknown stand %q", path, tierIndex, entryIndex, entry.Stand)
				}
			}
		}
	}
	return nil
}

func (c *AirlineAssignmentConfig) normalize() {
	groups := make(map[string][]string, len(c.StandGroups))
	for name, members := range c.StandGroups {
		cleanMembers := make([]string, 0, len(members))
		for _, member := range members {
			cleanMembers = append(cleanMembers, strings.TrimSpace(member))
		}
		groups[strings.TrimSpace(name)] = cleanMembers
	}
	c.StandGroups = groups
	groupNames := make(map[string]string, len(c.StandGroups))
	for name := range c.StandGroups {
		groupNames[normalizeGroupName(name)] = name
	}

	for ruleIndex := range c.Rules {
		rule := &c.Rules[ruleIndex]
		rule.ID = strings.TrimSpace(rule.ID)
		for i := range rule.Callsigns {
			rule.Callsigns[i] = normalizeCallsignPattern(rule.Callsigns[i])
		}
		normalizeConditions(rule.Conditions)
		normalizeStandMap(rule.Stands, groupNames)
	}
	groupKeys := make(map[string]struct{}, len(groupNames))
	for name := range groupNames {
		groupKeys[name] = struct{}{}
	}
	for ruleIndex := range c.Rules {
		c.Rules[ruleIndex].Tiers = tiersFromStandMap(c.Rules[ruleIndex].Stands, groupKeys)
	}
	for name, fallback := range c.FallbackRules {
		normalizeStandMap(fallback.Stands, groupNames)
		fallback.Tiers = tiersFromStandMap(fallback.Stands, groupKeys)
		c.FallbackRules[strings.TrimSpace(name)] = fallback
		if strings.TrimSpace(name) != name {
			delete(c.FallbackRules, name)
		}
	}
}

func normalizeStandMap(stands map[string]map[string]float64, groupNames map[string]string) {
	for tier, entries := range stands {
		cleanTier := strings.ToLower(strings.TrimSpace(tier))
		if cleanTier != tier {
			delete(stands, tier)
			stands[cleanTier] = entries
		}
		for name, weight := range entries {
			cleanName := normalizeStandName(name)
			if groupName, isGroup := groupNames[normalizeGroupName(name)]; isGroup {
				cleanName = groupName
			}
			if cleanName != name {
				delete(entries, name)
				entries[cleanName] += weight
			}
		}
	}
}

func tiersFromStandMap(stands map[string]map[string]float64, groupNames map[string]struct{}) []StandTier {
	if len(stands) == 0 {
		return nil
	}
	tierNames := make([]string, 0, len(stands))
	for name := range stands {
		tierNames = append(tierNames, name)
	}
	slices.SortStableFunc(tierNames, func(a, b string) int {
		return compareTierNames(a, b)
	})
	tiers := make([]StandTier, 0, len(tierNames))
	for _, name := range tierNames {
		entries := make([]WeightedStand, 0, len(stands[name]))
		for target, weight := range stands[name] {
			if _, isGroup := groupNames[normalizeGroupName(target)]; isGroup {
				entries = append(entries, WeightedStand{Group: target, Weight: weight})
			} else {
				entries = append(entries, WeightedStand{Stand: target, Weight: weight})
			}
		}
		slices.SortStableFunc(entries, func(a, b WeightedStand) int {
			return strings.Compare(a.Stand+a.Group, b.Stand+b.Group)
		})
		tiers = append(tiers, StandTier{Name: name, Entries: entries})
	}
	return tiers
}

func compareTierNames(a, b string) int {
	if a == b {
		return 0
	}
	if strings.HasPrefix(a, "tier") && strings.HasPrefix(b, "tier") {
		var aNumber, bNumber int
		if _, err := fmt.Sscanf(strings.TrimPrefix(a, "tier"), "%d", &aNumber); err == nil {
			if _, err := fmt.Sscanf(strings.TrimPrefix(b, "tier"), "%d", &bNumber); err == nil && aNumber != bNumber {
				if aNumber < bNumber {
					return -1
				}
				return 1
			}
		}
	}
	return strings.Compare(a, b)
}

func normalizeConditions(conditions *AirlineAssignmentConditions) {
	if conditions == nil {
		return
	}
	conditions.BorderStatus = BorderStatus(strings.ToUpper(strings.TrimSpace(string(conditions.BorderStatus))))
	conditions.Direction = AssignmentDirection(strings.ToUpper(strings.TrimSpace(string(conditions.Direction))))
	conditions.Special = strings.ToUpper(strings.TrimSpace(conditions.Special))
	for i := range conditions.AircraftTypes {
		conditions.AircraftTypes[i] = normalizeCallsignPattern(conditions.AircraftTypes[i])
	}
	for i := range conditions.AircraftUse {
		conditions.AircraftUse[i] = AircraftUseCode(strings.ToUpper(strings.TrimSpace(string(conditions.AircraftUse[i]))))
	}
}

func normalizeCallsignPattern(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeStandName(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeGroupName(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizeFallbackName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func lookupFallback(fallbacks map[string]FallbackRule, name string) (FallbackRule, bool) {
	for key, fallback := range fallbacks {
		if normalizeFallbackName(key) == normalizeFallbackName(name) {
			return fallback, true
		}
	}
	return FallbackRule{}, false
}

// ResolveStandGroup expands a group recursively and returns unique physical
// stand IDs in declaration order. Group references are case-insensitive.
func (c *AirlineAssignmentConfig) ResolveStandGroup(name string, registries ...*StandCapabilityRegistry) ([]string, error) {
	if c == nil {
		return nil, errors.New("airline assignment configuration is nil")
	}
	if len(registries) > 1 {
		return nil, errors.New("airline assignment accepts at most one stand registry")
	}
	registry := c.standRegistry
	if len(registries) == 1 {
		registry = registries[0]
	}
	groups := make(map[string][]string, len(c.StandGroups))
	for groupName, members := range c.StandGroups {
		groups[normalizeGroupName(groupName)] = members
	}
	requested := normalizeGroupName(name)
	if _, ok := groups[requested]; !ok {
		return nil, fmt.Errorf("unknown stand group %q", name)
	}
	visiting := make(map[string]bool)
	seen := make(map[string]bool)
	result := make([]string, 0)
	var expand func(string) error
	expand = func(group string) error {
		if visiting[group] {
			return fmt.Errorf("circular stand group reference involving %q", group)
		}
		visiting[group] = true
		defer delete(visiting, group)
		for _, member := range groups[group] {
			member = normalizeStandName(member)
			if _, isGroup := groups[member]; isGroup {
				if err := expand(member); err != nil {
					return err
				}
				continue
			}
			if registry != nil {
				if _, ok := registry.Lookup("EKCH", member); !ok {
					return fmt.Errorf("stand group %q references unknown stand %q", group, member)
				}
			}
			if !seen[member] {
				seen[member] = true
				result = append(result, member)
			}
		}
		return nil
	}
	if err := expand(requested); err != nil {
		return nil, err
	}
	return result, nil
}

// ExpandStandGroup is a concise alias for ResolveStandGroup.
func (c *AirlineAssignmentConfig) ExpandStandGroup(name string) ([]string, error) {
	return c.ResolveStandGroup(name)
}

// GetFallbackRule returns a fallback rule by its case-insensitive name.
func (c *AirlineAssignmentConfig) GetFallbackRule(name string) (FallbackRule, bool) {
	if c == nil {
		return FallbackRule{}, false
	}
	return lookupFallback(c.FallbackRules, name)
}

// MatchRule applies the documented precedence order. Ambiguous matches are an
// error instead of silently depending on JSON declaration order.
func (c *AirlineAssignmentConfig) MatchRule(facts AssignmentFlightFacts) (*RuleMatch, error) {
	if c == nil {
		return nil, errors.New("airline assignment configuration is nil")
	}
	facts.Callsign = normalizeCallsignPattern(facts.Callsign)
	facts.AircraftType = normalizeCallsignPattern(facts.AircraftType)
	facts.AircraftUse = AircraftUseCode(strings.ToUpper(strings.TrimSpace(string(facts.AircraftUse))))
	facts.BorderStatus = BorderStatus(strings.ToUpper(strings.TrimSpace(string(facts.BorderStatus))))
	facts.Direction = AssignmentDirection(strings.ToUpper(strings.TrimSpace(string(facts.Direction))))
	facts.Special = strings.ToUpper(strings.TrimSpace(facts.Special))

	type candidate struct {
		rule        *AirlineAssignmentRule
		pattern     string
		precedence  RulePrecedence
		specificity int
	}
	var candidates []candidate
	for i := range c.Rules {
		rule := &c.Rules[i]
		if !conditionsMatch(rule.Conditions, facts) {
			continue
		}
		if rule.Conditions != nil && rule.Conditions.Special != "" {
			if facts.Special != rule.Conditions.Special {
				continue
			}
			candidates = append(candidates, candidate{rule: rule, precedence: RulePrecedenceSpecial, specificity: len(rule.Conditions.Special)})
			continue
		}
		for _, pattern := range rule.Callsigns {
			precedence, specificity, ok := callsignMatch(pattern, facts.Callsign)
			if ok {
				candidates = append(candidates, candidate{rule: rule, pattern: pattern, precedence: precedence, specificity: specificity})
			}
		}
	}
	if len(candidates) > 0 {
		best := candidates[0]
		for _, candidate := range candidates[1:] {
			if precedenceRank(candidate.precedence) < precedenceRank(best.precedence) ||
				(precedenceRank(candidate.precedence) == precedenceRank(best.precedence) && candidate.specificity > best.specificity) {
				best = candidate
			}
		}
		matches := 0
		for _, candidate := range candidates {
			if candidate.precedence == best.precedence && candidate.specificity == best.specificity {
				matches++
			}
		}
		if matches > 1 {
			return nil, fmt.Errorf("ambiguous %s rule match for callsign %q", best.precedence, facts.Callsign)
		}
		return &RuleMatch{Rule: best.rule, Precedence: best.precedence, Pattern: best.pattern}, nil
	}

	if fallbackName := fallbackNameForFacts(facts); fallbackName != FallbackAirlinerDefault {
		if fallback, ok := c.GetFallbackRule(fallbackName); ok {
			rule := AirlineAssignmentRule{ID: fallbackName, Stands: fallback.Stands, Tiers: fallback.Tiers}
			return &RuleMatch{Rule: &rule, Fallback: fallbackName, Precedence: RulePrecedenceUseFallback}, nil
		}
	}
	fallback, ok := c.GetFallbackRule(FallbackAirlinerDefault)
	if !ok {
		return nil, errors.New("airliner_default fallback is not configured")
	}
	rule := AirlineAssignmentRule{ID: FallbackAirlinerDefault, Stands: fallback.Stands, Tiers: fallback.Tiers}
	return &RuleMatch{Rule: &rule, Fallback: FallbackAirlinerDefault, Precedence: RulePrecedenceDefaultFallback}, nil
}

func conditionsMatch(conditions *AirlineAssignmentConditions, facts AssignmentFlightFacts) bool {
	if conditions == nil {
		return true
	}
	if conditions.BorderStatus != "" && conditions.BorderStatus != facts.BorderStatus {
		return false
	}
	if conditions.Direction != "" && conditions.Direction != facts.Direction {
		return false
	}
	if conditions.Special != "" && conditions.Special != facts.Special {
		return false
	}
	if len(conditions.AircraftUse) > 0 && !slices.Contains(conditions.AircraftUse, facts.AircraftUse) {
		return false
	}
	if len(conditions.AircraftTypes) > 0 {
		matched := false
		for _, pattern := range conditions.AircraftTypes {
			if wildcardMatch(pattern, facts.AircraftType) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func callsignMatch(pattern, callsign string) (RulePrecedence, int, bool) {
	if pattern == callsign {
		return RulePrecedenceExactCallsign, len(pattern), true
	}
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return RulePrecedenceWildcard, len(prefix), strings.HasPrefix(callsign, prefix)
	}
	if pattern != "" && strings.HasPrefix(callsign, pattern) {
		return RulePrecedenceAirlinePrefix, len(pattern), true
	}
	return "", 0, false
}

func wildcardMatch(pattern, value string) bool {
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(value, strings.TrimSuffix(pattern, "*"))
	}
	return pattern == value
}

func precedenceRank(precedence RulePrecedence) int {
	switch precedence {
	case RulePrecedenceSpecial:
		return 0
	case RulePrecedenceExactCallsign:
		return 1
	case RulePrecedenceWildcard:
		return 2
	case RulePrecedenceAirlinePrefix:
		return 3
	default:
		return 99
	}
}

func fallbackNameForFacts(facts AssignmentFlightFacts) string {
	switch facts.AircraftUse {
	case AircraftUseCodeB:
		return FallbackBusinessVIP
	case AircraftUseCodeC:
		return FallbackCargo
	case AircraftUseCodeM:
		return FallbackMilitary
	case AircraftUseCodeH:
		return FallbackHelicopter
	case AircraftUseCodeP:
		return FallbackGAPrivate
	case AircraftUseCodeI:
		return FallbackMilitaryHelicopter
	case AircraftUseCodeA, AircraftUseCodeT:
		return FallbackAirlinerDefault
	default:
		return FallbackUnknown
	}
}
