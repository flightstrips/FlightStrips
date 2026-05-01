package config

import (
	"slices"
	"strings"
)

type ClxValidationConfig struct {
	SidFirstWaypoints   []string                `yaml:"sid_first_waypoints"`
	SidEngineRules      []ClxSidEngineRule      `yaml:"sid_engine_rules"`
	AircraftRunwayRules []ClxAircraftRunwayRule `yaml:"aircraft_runway_rules"`
	RnavRules           ClxRnavRules            `yaml:"rnav_rules"`
	RouteConflictRules  []ClxRouteConflictRule  `yaml:"route_conflict_rules"`
}

type ClxSidEngineRule struct {
	Code                  string            `yaml:"code"`
	SidFamilies           []string          `yaml:"sid_families"`
	DisallowedEngineTypes []string          `yaml:"disallowed_engine_types"`
	Message               string            `yaml:"message"`
	Recommendations       map[string]string `yaml:"recommendations"`
	DefaultRecommendation string            `yaml:"default_recommendation"`
}

type ClxAircraftRunwayRule struct {
	Code                  string   `yaml:"code"`
	AircraftTypes         []string `yaml:"aircraft_types"`
	RestrictedRunways     []string `yaml:"restricted_runways"`
	RestrictedSidSuffixes []string `yaml:"restricted_sid_suffixes"`
	Message               string   `yaml:"message"`
}

type ClxRnavRules struct {
	Nil          ClxRnavFaultRule      `yaml:"nil"`
	Insufficient ClxRnavCapabilityRule `yaml:"insufficient"`
}

type ClxRnavFaultRule struct {
	Code    string `yaml:"code"`
	Message string `yaml:"message"`
}

type ClxRnavCapabilityRule struct {
	Code         string   `yaml:"code"`
	Capabilities []string `yaml:"capabilities"`
	Message      string   `yaml:"message"`
}

type ClxRouteConflictRule struct {
	Code            string   `yaml:"code"`
	SidFamilies     []string `yaml:"sid_families"`
	RouteTokensAny  []string `yaml:"route_tokens_any"`
	RemarkTokensAny []string `yaml:"remark_tokens_any"`
	Always          bool     `yaml:"always"`
	Message         string   `yaml:"message"`
	AllowOverride   bool     `yaml:"allow_override"`
}

var clxValidationConfig ClxValidationConfig

func GetClxValidationConfig() ClxValidationConfig {
	return cloneClxValidationConfig(clxValidationConfig)
}

func (cfg ClxValidationConfig) IsEmpty() bool {
	return len(cfg.SidEngineRules) == 0 &&
		len(cfg.AircraftRunwayRules) == 0 &&
		clxRnavFaultRuleEmpty(cfg.RnavRules.Nil) &&
		clxRnavCapabilityRuleEmpty(cfg.RnavRules.Insufficient) &&
		len(cfg.RouteConflictRules) == 0
}

func normalizeClxValidationConfig(cfg ClxValidationConfig) ClxValidationConfig {
	return ClxValidationConfig{
		SidFirstWaypoints:   normalizeStringList(cfg.SidFirstWaypoints),
		SidEngineRules:      normalizeClxSidEngineRules(cfg.SidEngineRules),
		AircraftRunwayRules: normalizeClxAircraftRunwayRules(cfg.AircraftRunwayRules),
		RnavRules: ClxRnavRules{
			Nil: ClxRnavFaultRule{
				Code:    normalizeClxCode(cfg.RnavRules.Nil.Code),
				Message: strings.TrimSpace(cfg.RnavRules.Nil.Message),
			},
			Insufficient: ClxRnavCapabilityRule{
				Code:         normalizeClxCode(cfg.RnavRules.Insufficient.Code),
				Capabilities: normalizeStringList(cfg.RnavRules.Insufficient.Capabilities),
				Message:      strings.TrimSpace(cfg.RnavRules.Insufficient.Message),
			},
		},
		RouteConflictRules: normalizeClxRouteConflictRules(cfg.RouteConflictRules),
	}
}

func cloneClxValidationConfig(cfg ClxValidationConfig) ClxValidationConfig {
	return ClxValidationConfig{
		SidFirstWaypoints:   slices.Clone(cfg.SidFirstWaypoints),
		SidEngineRules:      cloneClxSidEngineRules(cfg.SidEngineRules),
		AircraftRunwayRules: cloneClxAircraftRunwayRules(cfg.AircraftRunwayRules),
		RnavRules: ClxRnavRules{
			Nil: ClxRnavFaultRule{
				Code:    cfg.RnavRules.Nil.Code,
				Message: cfg.RnavRules.Nil.Message,
			},
			Insufficient: ClxRnavCapabilityRule{
				Code:         cfg.RnavRules.Insufficient.Code,
				Capabilities: slices.Clone(cfg.RnavRules.Insufficient.Capabilities),
				Message:      cfg.RnavRules.Insufficient.Message,
			},
		},
		RouteConflictRules: cloneClxRouteConflictRules(cfg.RouteConflictRules),
	}
}

func normalizeClxSidEngineRules(rules []ClxSidEngineRule) []ClxSidEngineRule {
	if len(rules) == 0 {
		return nil
	}

	normalized := make([]ClxSidEngineRule, 0, len(rules))
	for _, rule := range rules {
		normalized = append(normalized, ClxSidEngineRule{
			Code:                  normalizeClxCode(rule.Code),
			SidFamilies:           normalizeStringList(rule.SidFamilies),
			DisallowedEngineTypes: normalizeStringList(rule.DisallowedEngineTypes),
			Message:               strings.TrimSpace(rule.Message),
			Recommendations:       normalizeClxRecommendations(rule.Recommendations),
			DefaultRecommendation: strings.TrimSpace(rule.DefaultRecommendation),
		})
	}

	return normalized
}

func cloneClxSidEngineRules(rules []ClxSidEngineRule) []ClxSidEngineRule {
	if len(rules) == 0 {
		return nil
	}

	cloned := make([]ClxSidEngineRule, 0, len(rules))
	for _, rule := range rules {
		cloned = append(cloned, ClxSidEngineRule{
			Code:                  rule.Code,
			SidFamilies:           slices.Clone(rule.SidFamilies),
			DisallowedEngineTypes: slices.Clone(rule.DisallowedEngineTypes),
			Message:               rule.Message,
			Recommendations:       cloneClxRecommendations(rule.Recommendations),
			DefaultRecommendation: rule.DefaultRecommendation,
		})
	}

	return cloned
}

func normalizeClxAircraftRunwayRules(rules []ClxAircraftRunwayRule) []ClxAircraftRunwayRule {
	if len(rules) == 0 {
		return nil
	}

	normalized := make([]ClxAircraftRunwayRule, 0, len(rules))
	for _, rule := range rules {
		normalized = append(normalized, ClxAircraftRunwayRule{
			Code:                  normalizeClxCode(rule.Code),
			AircraftTypes:         normalizeStringList(rule.AircraftTypes),
			RestrictedRunways:     normalizeStringList(rule.RestrictedRunways),
			RestrictedSidSuffixes: normalizeStringList(rule.RestrictedSidSuffixes),
			Message:               strings.TrimSpace(rule.Message),
		})
	}

	return normalized
}

func cloneClxAircraftRunwayRules(rules []ClxAircraftRunwayRule) []ClxAircraftRunwayRule {
	if len(rules) == 0 {
		return nil
	}

	cloned := make([]ClxAircraftRunwayRule, 0, len(rules))
	for _, rule := range rules {
		cloned = append(cloned, ClxAircraftRunwayRule{
			Code:                  rule.Code,
			AircraftTypes:         slices.Clone(rule.AircraftTypes),
			RestrictedRunways:     slices.Clone(rule.RestrictedRunways),
			RestrictedSidSuffixes: slices.Clone(rule.RestrictedSidSuffixes),
			Message:               rule.Message,
		})
	}

	return cloned
}

func normalizeClxRouteConflictRules(rules []ClxRouteConflictRule) []ClxRouteConflictRule {
	if len(rules) == 0 {
		return nil
	}

	normalized := make([]ClxRouteConflictRule, 0, len(rules))
	for _, rule := range rules {
		normalized = append(normalized, ClxRouteConflictRule{
			Code:            normalizeClxCode(rule.Code),
			SidFamilies:     normalizeStringList(rule.SidFamilies),
			RouteTokensAny:  normalizeStringList(rule.RouteTokensAny),
			RemarkTokensAny: normalizeStringList(rule.RemarkTokensAny),
			Always:          rule.Always,
			Message:         strings.TrimSpace(rule.Message),
			AllowOverride:   rule.AllowOverride,
		})
	}

	return normalized
}

func cloneClxRouteConflictRules(rules []ClxRouteConflictRule) []ClxRouteConflictRule {
	if len(rules) == 0 {
		return nil
	}

	cloned := make([]ClxRouteConflictRule, 0, len(rules))
	for _, rule := range rules {
		cloned = append(cloned, ClxRouteConflictRule{
			Code:            rule.Code,
			SidFamilies:     slices.Clone(rule.SidFamilies),
			RouteTokensAny:  slices.Clone(rule.RouteTokensAny),
			RemarkTokensAny: slices.Clone(rule.RemarkTokensAny),
			Always:          rule.Always,
			Message:         rule.Message,
			AllowOverride:   rule.AllowOverride,
		})
	}

	return cloned
}

func normalizeClxRecommendations(recommendations map[string]string) map[string]string {
	if len(recommendations) == 0 {
		return nil
	}

	normalized := make(map[string]string, len(recommendations))
	for token, recommendation := range recommendations {
		normalizedToken := strings.ToUpper(strings.TrimSpace(token))
		normalizedRecommendation := strings.TrimSpace(recommendation)
		if normalizedToken == "" || normalizedRecommendation == "" {
			continue
		}
		normalized[normalizedToken] = normalizedRecommendation
	}

	if len(normalized) == 0 {
		return nil
	}

	return normalized
}

func cloneClxRecommendations(recommendations map[string]string) map[string]string {
	if len(recommendations) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(recommendations))
	for token, recommendation := range recommendations {
		cloned[token] = recommendation
	}

	return cloned
}

func normalizeClxCode(code string) string {
	return strings.TrimSpace(code)
}

func clxRnavFaultRuleEmpty(rule ClxRnavFaultRule) bool {
	return rule.Code == "" && rule.Message == ""
}

func clxRnavCapabilityRuleEmpty(rule ClxRnavCapabilityRule) bool {
	return rule.Code == "" && len(rule.Capabilities) == 0 && rule.Message == ""
}
