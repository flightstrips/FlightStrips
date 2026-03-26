package cdm

import (
	"FlightStrips/pkg/models"
	"strconv"
	"strings"
)

const (
	DefaultCDMRate        = 20
	DefaultCDMRateLVO     = 14
	DefaultCDMTaxiMinutes = 10
)

type CdmRate struct {
	Airport      string   `json:"airport"`
	ArrRwyYes    []string `json:"arrRwyYes"`
	ArrRwyNo     []string `json:"arrRwyNo"`
	DepRwyYes    []string `json:"depRwyYes"`
	DepRwyNo     []string `json:"depRwyNo"`
	DependentRwy []string `json:"dependentRwy"`
	Rates        []string `json:"rates"`
	RatesLvo     []string `json:"ratesLvo"`
}

type CdmSidInterval struct {
	Airport string  `json:"airport"`
	Runway  string  `json:"rwy"`
	Sid1    string  `json:"sid1"`
	Sid2    string  `json:"sid2"`
	Value   float64 `json:"value"`
}

type CdmTaxiPoint struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

type CdmTaxiZone struct {
	Airport           string         `json:"airport"`
	Runway            string         `json:"runway"`
	Minutes           int            `json:"minutes"`
	Polygon           []CdmTaxiPoint `json:"polygon,omitempty"`
	RemoteTaxiMinutes []int          `json:"remoteTaxiMinutes,omitempty"`
}

type CdmDelay struct {
	Airport string `json:"airport"`
	Runway  string `json:"runway"`
	Time    string `json:"time"`
	Type    string `json:"type"`
}

type CdmDeicePlatformConfig struct {
	Name string `json:"name"`
	Time int    `json:"time"`
}

type CdmDeiceConfig struct {
	Light    int                      `json:"light"`
	Medium   int                      `json:"medium"`
	Heavy    int                      `json:"heavy"`
	Super    int                      `json:"super"`
	Platform []CdmDeicePlatformConfig `json:"platform"`
}

type CdmAirportConfig struct {
	Airport                string           `json:"airport"`
	Rates                  []CdmRate        `json:"rates"`
	SidIntervals           []CdmSidInterval `json:"sidIntervals"`
	TaxiZones              []CdmTaxiZone    `json:"taxiZones"`
	Delays                 []CdmDelay       `json:"delays"`
	DefaultRate            int              `json:"defaultRate"`
	DefaultRateLvo         int              `json:"defaultRateLvo"`
	DefaultTaxiMinutes     int              `json:"defaultTaxiMinutes"`
	LvoActive              bool             `json:"lvoActive"`
	ActiveArrivalRunways   []string         `json:"activeArrivalRunways,omitempty"`
	ActiveDepartureRunways []string         `json:"activeDepartureRunways,omitempty"`
	DeiceConfig            CdmDeiceConfig   `json:"deiceConfig"`
}

func NewDefaultAirportConfig(airport string) *CdmAirportConfig {
	return &CdmAirportConfig{
		Airport:            normalizeToken(airport),
		DefaultRate:        DefaultCDMRate,
		DefaultRateLvo:     DefaultCDMRateLVO,
		DefaultTaxiMinutes: DefaultCDMTaxiMinutes,
	}
}

func (c *CdmAirportConfig) Clone() *CdmAirportConfig {
	if c == nil {
		return nil
	}

	clone := *c
	clone.Rates = append([]CdmRate(nil), c.Rates...)
	clone.SidIntervals = append([]CdmSidInterval(nil), c.SidIntervals...)
	clone.TaxiZones = append([]CdmTaxiZone(nil), c.TaxiZones...)
	clone.Delays = append([]CdmDelay(nil), c.Delays...)
	clone.ActiveArrivalRunways = append([]string(nil), c.ActiveArrivalRunways...)
	clone.ActiveDepartureRunways = append([]string(nil), c.ActiveDepartureRunways...)
	clone.DeiceConfig.Platform = append([]CdmDeicePlatformConfig(nil), c.DeiceConfig.Platform...)
	return &clone
}

func (c *CdmAirportConfig) WithActiveRunways(active models.ActiveRunways) *CdmAirportConfig {
	if active.ArrivalRunways == nil && active.DepartureRunways == nil {
		return c.Clone()
	}
	return c.SnapshotWithRunways(active.ArrivalRunways, active.DepartureRunways)
}

func (c *CdmAirportConfig) SnapshotWithRunways(arrivals, departures []string) *CdmAirportConfig {
	clone := c.Clone()
	if clone == nil {
		return nil
	}

	clone.ActiveArrivalRunways = append([]string(nil), arrivals...)
	clone.ActiveDepartureRunways = append([]string(nil), departures...)
	return clone
}

func (c *CdmAirportConfig) RateForRunway(depRwy string) int {
	if c == nil {
		return DefaultCDMRate
	}

	defaultRate := c.DefaultRate
	if defaultRate <= 0 {
		defaultRate = DefaultCDMRate
	}
	if c.LvoActive {
		defaultRate = c.DefaultRateLvo
		if defaultRate <= 0 {
			defaultRate = DefaultCDMRateLVO
		}
	}

	for _, rate := range c.Rates {
		if !strings.EqualFold(rate.Airport, c.Airport) {
			continue
		}
		if !rate.matches(depRwy, c.ActiveArrivalRunways, c.ActiveDepartureRunways) {
			continue
		}

		values := rate.Rates
		if c.LvoActive && len(rate.RatesLvo) > 0 {
			values = rate.RatesLvo
		}
		if len(values) == 0 {
			return defaultRate
		}

		index := rate.valueIndex(depRwy, len(values))
		if parsed, ok := parsePositiveInt(values[index]); ok {
			return parsed
		}
		return defaultRate
	}

	return defaultRate
}

func (c *CdmAirportConfig) TaxiMinutesForRunway(depRwy string) int {
	if c == nil {
		return DefaultCDMTaxiMinutes
	}

	for _, zone := range c.TaxiZones {
		if strings.EqualFold(zone.Runway, depRwy) && zone.Minutes > 0 {
			return zone.Minutes
		}
	}

	if c.DefaultTaxiMinutes > 0 {
		return c.DefaultTaxiMinutes
	}
	return DefaultCDMTaxiMinutes
}

func (c *CdmAirportConfig) SidIntervalMinutes(depRwy, sid1, sid2 string) float64 {
	if c == nil {
		return 0
	}

	if sameSID(sid1, sid2) {
		return 0
	}

	for _, interval := range c.SidIntervals {
		if !strings.EqualFold(interval.Runway, depRwy) {
			continue
		}
		if sameSID(interval.Sid1, sid1) && sameSID(interval.Sid2, sid2) {
			return interval.Value
		}
		if sameSID(interval.Sid1, sid2) && sameSID(interval.Sid2, sid1) {
			return interval.Value
		}
	}

	return 0
}

func (c *CdmAirportConfig) DependentRunways(depRwy string) []string {
	if c == nil {
		return nil
	}

	for _, rate := range c.Rates {
		if !rate.depRwyMatches(depRwy) {
			continue
		}
		if len(rate.DependentRwy) == 0 || isWildcardList(rate.DependentRwy) {
			return nil
		}
		return append([]string(nil), rate.DependentRwy...)
	}

	return nil
}

func (r CdmRate) matches(depRwy string, activeArrivals, activeDepartures []string) bool {
	if !r.depRwyMatches(depRwy) {
		return false
	}

	if !listMatchesActive(r.ArrRwyYes, activeArrivals, true) {
		return false
	}
	if !listExcludesActive(r.ArrRwyNo, activeArrivals) {
		return false
	}
	if !listMatchesActive(r.DepRwyYes, activeDepartures, true) {
		return false
	}
	if !listExcludesActive(r.DepRwyNo, activeDepartures) {
		return false
	}
	return true
}

func (r CdmRate) depRwyMatches(depRwy string) bool {
	if len(r.DepRwyYes) == 0 {
		return false
	}
	for _, runway := range r.DepRwyYes {
		if runway == "*" || strings.EqualFold(runway, depRwy) {
			return true
		}
	}
	return false
}

func (r CdmRate) valueIndex(depRwy string, values int) int {
	if values <= 1 {
		return 0
	}

	for idx, runway := range r.DepRwyYes {
		if idx >= values {
			break
		}
		if strings.EqualFold(runway, depRwy) {
			return idx
		}
	}
	return 0
}

func parsePositiveInt(value string) (int, bool) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || parsed <= 0 {
		return 0, false
	}
	return parsed, true
}

func sameRunway(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}

func sameSID(a, b string) bool {
	left := sidVariants(a)
	right := sidVariants(b)
	for _, l := range left {
		for _, r := range right {
			if l != "" && l == r {
				return true
			}
		}
	}
	return false
}

func sidVariants(value string) []string {
	trimmed := strings.ToUpper(strings.TrimSpace(value))
	if trimmed == "" {
		return nil
	}

	variants := []string{trimmed}
	if len(trimmed) > 3 {
		variants = append(variants, trimmed[:len(trimmed)-2])
	}
	return variants
}

func listMatchesActive(configured, active []string, emptyIsMatch bool) bool {
	if len(configured) == 0 {
		return emptyIsMatch
	}
	if isWildcardList(configured) {
		return true
	}
	if len(active) == 0 {
		return true
	}

	for _, configuredValue := range configured {
		for _, activeValue := range active {
			if sameRunway(configuredValue, activeValue) {
				return true
			}
		}
	}
	return false
}

func listExcludesActive(configured, active []string) bool {
	if len(configured) == 0 || isWildcardList(configured) || len(active) == 0 {
		return true
	}

	for _, configuredValue := range configured {
		for _, activeValue := range active {
			if sameRunway(configuredValue, activeValue) {
				return false
			}
		}
	}
	return true
}

func isWildcardList(values []string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == "*" {
			return true
		}
	}
	return false
}

// DeiceMinutesForWtc returns the deice taxi time (in minutes) for the given WTC category.
// WTC is matched case-insensitively against "L", "M", "H", "J". Returns 0 for unknown categories.
func (c *CdmAirportConfig) DeiceMinutesForWtc(wtc string) int {
	if c == nil {
		return 0
	}
	switch strings.ToUpper(strings.TrimSpace(wtc)) {
	case "L":
		return c.DeiceConfig.Light
	case "M":
		return c.DeiceConfig.Medium
	case "H":
		return c.DeiceConfig.Heavy
	case "J":
		return c.DeiceConfig.Super
	}
	return 0
}

// DeiceMinutesForPlatform returns the platform-specific deice time (in minutes) and true if found,
// or (0, false) if no platform with that name is configured.
func (c *CdmAirportConfig) DeiceMinutesForPlatform(platform string) (int, bool) {
	if c == nil {
		return 0, false
	}
	for _, p := range c.DeiceConfig.Platform {
		if strings.EqualFold(p.Name, platform) {
			return p.Time, true
		}
	}
	return 0, false
}

func normalizeToken(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
