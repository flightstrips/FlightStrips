package config

import (
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v4"
)

// Stand represents a single stand like "A15".
type Stand struct {
	Prefix string `yaml:"prefix"`
	Number int    `yaml:"number"`
}

// Equal compares two stands for equality.
func (s Stand) Equal(o Stand) bool {
	return strings.EqualFold(s.Prefix, o.Prefix) && s.Number == o.Number
}

// ParseStand parses strings like "A15" into a Stand.
func ParseStand(s string) (Stand, error) {
	str := strings.TrimSpace(s)
	if str == "" {
		return Stand{}, fmt.Errorf("empty stand")
	}
	// Split into leading letters and trailing digits (ASCII-oriented).
	i := 0
	for i < len(str) && (str[i] < '0' || str[i] > '9') {
		i++
	}
	if i == 0 || i == len(str) {
		return Stand{}, fmt.Errorf("invalid stand format: %q", s)
	}
	prefix := strings.ToUpper(str[:i])
	numStr := str[i:]
	n, err := strconv.Atoi(numStr)
	if err != nil {
		return Stand{}, fmt.Errorf("invalid stand number %q: %w", numStr, err)
	}
	return Stand{Prefix: prefix, Number: n}, nil
}

// StandRange represents a contiguous range of stands (e.g., A15-A30).
type StandRange struct {
	Prefix string `yaml:"prefix"`
	From   int    `yaml:"from"`
	To     int    `yaml:"to"`
}

// UnmarshalYAML allows StandRange to be specified as:
// - scalar "A15-A30" (range)
// - scalar "A15" (single stand)
// - mapping {prefix: "A", from: 15, to: 30}
func (r *StandRange) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		str := strings.TrimSpace(value.Value)
		if strings.Contains(str, "-") {
			sr, err := ParseStandRange(str)
			if err != nil {
				return err
			}
			*r = sr
			return nil
		}
		// treat single stand scalar as From==To
		st, err := ParseStand(str)
		if err != nil {
			return err
		}
		*r = SingleStandRange(st.Prefix, st.Number)
		return nil
	case yaml.MappingNode:
		var aux struct {
			Prefix string `yaml:"prefix"`
			From   int    `yaml:"from"`
			To     int    `yaml:"to"`
		}
		if err := value.Decode(&aux); err != nil {
			return err
		}
		if strings.TrimSpace(aux.Prefix) == "" {
			return fmt.Errorf("stand range requires non-empty prefix")
		}
		if aux.From == 0 && aux.To == 0 {
			return fmt.Errorf("stand range requires at least one bound")
		}
		from, to := aux.From, aux.To
		if to == 0 {
			to = from
		}
		if from == 0 {
			from = to
		}
		if from > to {
			from, to = to, from
		}
		r.Prefix = strings.ToUpper(aux.Prefix)
		r.From = from
		r.To = to
		return nil
	default:
		return fmt.Errorf("invalid YAML for StandRange")
	}
}

// Contains returns true if the stand falls within the range.
func (r *StandRange) Contains(s Stand) bool {
	if !strings.EqualFold(r.Prefix, s.Prefix) {
		return false
	}
	return s.Number >= r.From && s.Number <= r.To
}

// ParseStandRange parses strings like "A15-A30" into a StandRange.
func ParseStandRange(s string) (StandRange, error) {
	str := strings.TrimSpace(s)
	parts := strings.Split(str, "-")
	if len(parts) != 2 {
		return StandRange{}, fmt.Errorf("invalid stand range format: %q", s)
	}
	left, err := ParseStand(parts[0])
	if err != nil {
		return StandRange{}, fmt.Errorf("left bound: %w", err)
	}
	right, err := ParseStand(parts[1])
	if err != nil {
		return StandRange{}, fmt.Errorf("right bound: %w", err)
	}
	if !strings.EqualFold(left.Prefix, right.Prefix) {
		return StandRange{}, fmt.Errorf("stand range prefixes must match: %q vs %q", left.Prefix, right.Prefix)
	}
	from := left.Number
	to := right.Number
	if from > to {
		from, to = to, from
	}
	return StandRange{Prefix: strings.ToUpper(left.Prefix), From: from, To: to}, nil
}

// SingleStandRange returns a StandRange representing a single stand (From == To).
func SingleStandRange(prefix string, number int) StandRange {
	return StandRange{Prefix: strings.ToUpper(prefix), From: number, To: number}
}

// Route is a simple, hard-coded taxi route defined by either a destination runway
// or a destination set of stands/stand ranges. Exactly one of ForRunway or ForStandRanges must be set.
// Note: A single stand is represented as a range with From == To (e.g., W1 -> {Prefix: "W", From: 1, To: 1}).
type Route struct {
	Name           string       `yaml:"name"`
	ForRunway      string       `yaml:"forRunway"`      // e.g., "RWY27" or "27" (case-insensitive)
	ForStandRanges []StandRange `yaml:"forStandRanges"` // e.g., A10-A20, B5-B10, or single stands like W1 (From=To=1)
	Path           []string     `yaml:"path"`           // ordered Sector names from general origin area to destination
	Active         []string     `yaml:"active"`         // all of these "active" flags must be present to use this route
}

// ComputeToRunway selects a route to the given runway that is valid under the current
// active Sector flags and contains the aircraft's current Sector within its path.
// Returns the subsequence of config from the current Sector to the end of the route.
func ComputeToRunway(active []string, currentSector string, runway string) ([]string, bool) {
	candidates := runwayRoutes[normalizeRunway(runway)]
	if len(candidates) == 0 {
		return nil, false
	}
	for _, r := range candidates {
		if !hasAllActive(active, r.Active) {
			continue
		}
		startIdx := indexOfSector(r.Path, currentSector)
		if startIdx < 0 {
			continue
		}
		return r.Path[startIdx:], true
	}
	return nil, false
}

// ComputeToStand selects a route to the given destination stand that is valid under
// current active flags and contains the aircraft's current Sector within its path.
// Returns the subsequence of config from the current Sector to the end of the route.
// Note: Single stands are represented as ranges with From == To.
func ComputeToStand(active []string, currentSector string, standStr string) ([]string, bool) {
	st, err := ParseStand(standStr)
	if err != nil {
		return nil, false
	}
	for _, r := range standRoutes {
		if len(r.ForStandRanges) == 0 {
			continue
		}
		// must match at least one configured range (including singletons)
		matched := false
		for _, sr := range r.ForStandRanges {
			if sr.Contains(st) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if !hasAllActive(active, r.Active) {
			continue
		}
		startIdx := indexOfSector(r.Path, currentSector)
		if startIdx < 0 {
			continue
		}
		// Without coverage mapping, the route's Path is assumed to lead to the stands;
		// return from current Sector to the end of the path.
		return r.Path[startIdx:], true
	}
	return nil, false
}

func normalizeRunway(rwy string) string {
	return strings.ToUpper(strings.TrimSpace(rwy))
}

func hasAllActive(active []string, required []string) bool {
	if len(required) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(active))
	for _, a := range active {
		set[strings.ToLower(strings.TrimSpace(a))] = struct{}{}
	}
	for _, req := range required {
		if _, ok := set[strings.ToLower(strings.TrimSpace(req))]; !ok {
			return false
		}
	}
	return true
}

func indexOfSector(path []string, sector string) int {
	for i, s := range path {
		if strings.EqualFold(s, sector) {
			return i
		}
	}
	return -1
}
