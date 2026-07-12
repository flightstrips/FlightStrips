package sat

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
)

// StandBorderClass describes the border class attached to a stand variant.
// An empty value means that the variant applies to either border class.
type StandBorderClass string

const (
	StandBorderAny         StandBorderClass = ""
	StandBorderSchengen    StandBorderClass = "SCHENGEN"
	StandBorderNonSchengen StandBorderClass = "NON-SCHENGEN"
)

// StandCapability is one GRPlugin STAND record and its capability directives.
// Preference directives such as PRIORITY, USE, CALLSIGN, and route rules are
// intentionally not represented here: they do not describe stand capability.
type StandCapability struct {
	Airport string
	Stand   string

	Latitude  float64
	Longitude float64
	Radius    float64

	Blocks []string

	BorderClass    StandBorderClass
	WTC            []string
	NotWTC         []string
	EngineTypes    []string
	NotEngineTypes []string

	Wingspan float64
	Length   float64
	Width    float64
	Height   float64
	MTOW     float64
	Code     string

	AircraftTypes    []string
	NotAircraftTypes []string

	Manual bool
	Areas  []string

	Line int
}

// Stand is the physical stand identity and all of its capability variants.
// Variants are kept in source order, including duplicate STAND records.
type Stand struct {
	Airport string
	Name    string

	Latitude  float64
	Longitude float64
	Radius    float64

	Blocks   []string
	Variants []StandCapability
}

// StandCapabilityRegistry is a validated, read-only index of physical stands.
type StandCapabilityRegistry struct {
	byAirport map[string]map[string]Stand
	ordered   []Stand
}

// LoadStandCapabilities parses GRpluginStands.txt data and validates its
// capability records, physical stand geometry, and block references.
func LoadStandCapabilities(source io.Reader) (*StandCapabilityRegistry, error) {
	if source == nil {
		return nil, errors.New("stand capability source is nil")
	}

	registry := &StandCapabilityRegistry{
		byAirport: make(map[string]map[string]Stand),
	}
	var problems []error
	var current *StandCapability
	var knownDirectives = map[string]struct{}{
		"PRIORITY": {}, "USE": {}, "CALLSIGN": {}, "NOTCALLSIGN": {},
		"ADEP": {}, "NOTADEP": {}, "DEP": {}, "NOTDEP": {},
		"ARR": {}, "NOTARR": {}, "ROUTE": {}, "NOTROUTE": {},
		"STANDLIST": {},
	}

	addCurrent := func() {
		if current == nil {
			return
		}
		variant := cloneStandCapability(*current)
		stand := Stand{
			Airport:   current.Airport,
			Name:      current.Stand,
			Latitude:  current.Latitude,
			Longitude: current.Longitude,
			Radius:    current.Radius,
			Blocks:    slices.Clone(current.Blocks),
			Variants:  []StandCapability{variant},
		}

		airportStands := registry.byAirport[stand.Airport]
		if airportStands == nil {
			airportStands = make(map[string]Stand)
			registry.byAirport[stand.Airport] = airportStands
		}
		if existing, ok := airportStands[stand.Name]; ok {
			if existing.Latitude != stand.Latitude || existing.Longitude != stand.Longitude || existing.Radius != stand.Radius {
				problems = append(problems, fmt.Errorf("line %d: conflicting geometry for stand %s:%s (first declared on line %d)", stand.Variants[0].Line, stand.Airport, stand.Name, existing.Variants[0].Line))
			}
			existing.Blocks = unionTokens(existing.Blocks, stand.Blocks)
			existing.Variants = append(existing.Variants, stand.Variants[0])
			airportStands[stand.Name] = existing
			for i := range registry.ordered {
				if registry.ordered[i].Airport == stand.Airport && registry.ordered[i].Name == stand.Name {
					registry.ordered[i] = existing
					break
				}
			}
			return
		}
		airportStands[stand.Name] = stand
		registry.ordered = append(registry.ordered, stand)
	}

	scanner := bufio.NewScanner(source)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";;") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}

		key, value, hasValue := splitDirective(line)
		if key == "STAND" {
			addCurrent()
			capability, err := parseStandHeader(value, lineNumber)
			if err != nil {
				problems = append(problems, err)
				current = nil
				continue
			}
			current = &capability
			continue
		}

		if !hasValue && key == "MANUAL" {
			if current == nil {
				problems = append(problems, fmt.Errorf("line %d: MANUAL appears before a STAND record", lineNumber))
				continue
			}
			current.Manual = true
			continue
		}
		if !hasValue && (key == "SCHENGEN" || key == "NON-SCHENGEN") {
			if current == nil {
				problems = append(problems, fmt.Errorf("line %d: directive %q appears before a STAND record", lineNumber, key))
				continue
			}
			if current.BorderClass != StandBorderAny && current.BorderClass != StandBorderClass(key) {
				problems = append(problems, fmt.Errorf("line %d: conflicting border directives %q and %q", lineNumber, current.BorderClass, key))
				continue
			}
			current.BorderClass = StandBorderClass(key)
			continue
		}
		if current == nil {
			if key == "STANDLIST" {
				continue
			}
			problems = append(problems, fmt.Errorf("line %d: directive %q appears before a STAND record", lineNumber, key))
			continue
		}

		if _, ignored := knownDirectives[key]; ignored {
			continue
		}
		if !hasValue {
			problems = append(problems, fmt.Errorf("line %d: malformed directive %q: expected a value", lineNumber, key))
			continue
		}
		if err := applyStandDirective(current, key, value, lineNumber); err != nil {
			problems = append(problems, err)
		}
	}
	if err := scanner.Err(); err != nil {
		problems = append(problems, fmt.Errorf("read stand capability data: %w", err))
	}
	addCurrent()

	if len(registry.ordered) == 0 {
		problems = append(problems, errors.New("no STAND records found"))
	}
	for i := range registry.ordered {
		stand := &registry.ordered[i]
		stand.Blocks = filterKnownBlocks(stand.Airport, stand.Blocks, registry.byAirport)
		for j := range stand.Variants {
			stand.Variants[j].Blocks = filterKnownBlocks(stand.Airport, stand.Variants[j].Blocks, registry.byAirport)
		}
		registry.byAirport[stand.Airport][stand.Name] = *stand
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid stand capability data: %w", errors.Join(problems...))
	}
	return registry, nil
}

// LoadStandCapabilityFile opens and parses a committed stand capability file.
func LoadStandCapabilityFile(path string) (*StandCapabilityRegistry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open stand capability file %q: %w", path, err)
	}
	defer f.Close()
	registry, err := LoadStandCapabilities(f)
	if err != nil {
		return nil, fmt.Errorf("load stand capability file %q: %w", path, err)
	}
	return registry, nil
}

// Lookup returns a physical stand by airport and stand name.
func (r *StandCapabilityRegistry) Lookup(airport, name string) (Stand, bool) {
	if r == nil {
		return Stand{}, false
	}
	stand, ok := r.byAirport[normalizeToken(airport)][normalizeToken(name)]
	if !ok {
		return Stand{}, false
	}
	return cloneStand(stand), true
}

// Stands returns all physical stands at an airport in source order.
func (r *StandCapabilityRegistry) Stands(airport string) []Stand {
	if r == nil {
		return nil
	}
	result := make([]Stand, 0)
	for _, stand := range r.ordered {
		if stand.Airport == normalizeToken(airport) {
			result = append(result, cloneStand(stand))
		}
	}
	return result
}

// AllStands returns every physical stand in source order.
func (r *StandCapabilityRegistry) AllStands() []Stand {
	if r == nil {
		return nil
	}
	result := make([]Stand, 0, len(r.ordered))
	for _, stand := range r.ordered {
		result = append(result, cloneStand(stand))
	}
	return result
}

func parseStandHeader(value string, line int) (StandCapability, error) {
	parts := strings.Split(value, ":")
	if len(parts) != 5 {
		return StandCapability{}, fmt.Errorf("line %d: malformed STAND record: expected airport, stand, latitude, longitude, and radius", line)
	}
	airport := normalizeToken(parts[0])
	name := normalizeToken(parts[1])
	if airport == "" || name == "" {
		return StandCapability{}, fmt.Errorf("line %d: malformed STAND record: airport and stand are required", line)
	}
	latitude, err := parseGRCoordinate(parts[2], true)
	if err != nil {
		return StandCapability{}, fmt.Errorf("line %d: invalid latitude %q: %w", line, strings.TrimSpace(parts[2]), err)
	}
	longitude, err := parseGRCoordinate(parts[3], false)
	if err != nil {
		return StandCapability{}, fmt.Errorf("line %d: invalid longitude %q: %w", line, strings.TrimSpace(parts[3]), err)
	}
	radius, err := parseNonNegativeNumber("radius", parts[4])
	if err != nil {
		return StandCapability{}, fmt.Errorf("line %d: %w", line, err)
	}
	return StandCapability{Airport: airport, Stand: name, Latitude: latitude, Longitude: longitude, Radius: radius, Line: line}, nil
}

func applyStandDirective(capability *StandCapability, key, value string, line int) error {
	value = strings.TrimSpace(value)
	if value == "" && key != "AREA" && key != "CODE" {
		return fmt.Errorf("line %d: directive %s requires a value", line, key)
	}
	switch key {
	case "BLOCKS":
		blocks := parseBlocks(value)
		if len(blocks) == 0 {
			return fmt.Errorf("line %d: BLOCKS requires at least one stand", line)
		}
		capability.Blocks = unionTokens(capability.Blocks, blocks)
	case "SCHENGEN":
		return fmt.Errorf("line %d: SCHENGEN must be a flag without a value", line)
	case "NON-SCHENGEN":
		return fmt.Errorf("line %d: NON-SCHENGEN must be a flag without a value", line)
	case "WTC":
		capability.WTC = unionTokens(capability.WTC, parseTokenList(value))
	case "NOTWTC":
		capability.NotWTC = unionTokens(capability.NotWTC, parseTokenList(value))
	case "ENGINETYPE":
		capability.EngineTypes = unionTokens(capability.EngineTypes, parseTokenList(value))
	case "NOTENGINETYPE":
		capability.NotEngineTypes = unionTokens(capability.NotEngineTypes, parseTokenList(value))
	case "WINGSPAN":
		return setDimension(&capability.Wingspan, "WINGSPAN", value, line)
	case "LENGTH":
		return setDimension(&capability.Length, "LENGTH", value, line)
	case "WIDTH":
		return setDimension(&capability.Width, "WIDTH", value, line)
	case "HEIGHT":
		return setDimension(&capability.Height, "HEIGHT", value, line)
	case "MTOW":
		return setDimension(&capability.MTOW, "MTOW", value, line)
	case "CODE":
		capability.Code = normalizeToken(value)
	case "ATYP":
		capability.AircraftTypes = unionTokens(capability.AircraftTypes, parseTokenList(value))
	case "NOTATYP":
		capability.NotAircraftTypes = unionTokens(capability.NotAircraftTypes, parseTokenList(value))
	case "AREA":
		capability.Areas = unionTokens(capability.Areas, parseTokenList(value))
	case "MANUAL":
		return fmt.Errorf("line %d: MANUAL must be a flag without a value", line)
	default:
		return fmt.Errorf("line %d: unknown stand capability directive %q", line, key)
	}
	return nil
}

func splitDirective(line string) (string, string, bool) {
	parts := strings.SplitN(line, ":", 2)
	key := normalizeToken(parts[0])
	if len(parts) == 1 {
		return key, "", false
	}
	return key, parts[1], true
}

func parseBlocks(value string) []string {
	// GRPlugin may append a time qualifier, for example BLOCKS:5,6:36.
	if qualifier := strings.IndexByte(value, ':'); qualifier >= 0 {
		value = value[:qualifier]
	}
	return parseTokenList(value)
}

func parseTokenList(value string) []string {
	result := make([]string, 0)
	for _, token := range strings.Split(value, ",") {
		token = normalizeToken(token)
		if token != "" && !slices.Contains(result, token) {
			result = append(result, token)
		}
	}
	return result
}

func unionTokens(first, second []string) []string {
	result := slices.Clone(first)
	for _, token := range second {
		if token != "" && !slices.Contains(result, token) {
			result = append(result, token)
		}
	}
	return result
}

func filterKnownBlocks(airport string, blocks []string, stands map[string]map[string]Stand) []string {
	known := stands[airport]
	result := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if _, ok := known[block]; ok {
			result = append(result, block)
		}
	}
	return result
}

func setDimension(target *float64, name, value string, line int) error {
	number, err := parseNonNegativeNumber(name, value)
	if err != nil {
		return fmt.Errorf("line %d: %w", line, err)
	}
	*target = number
	return nil
}

func parseNonNegativeNumber(name, value string) (float64, error) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number < 0 {
		return 0, fmt.Errorf("invalid %s %q", name, strings.TrimSpace(value))
	}
	return number, nil
}

func parseGRCoordinate(value string, latitude bool) (float64, error) {
	value = strings.ToUpper(strings.TrimSpace(value))
	if value == "" {
		return 0, errors.New("coordinate is empty")
	}
	direction := value[0]
	if (latitude && direction != 'N' && direction != 'S') || (!latitude && direction != 'E' && direction != 'W') {
		return 0, fmt.Errorf("invalid direction %q", direction)
	}
	body := value[1:]
	parts := strings.Split(body, ".")
	if len(parts) != 3 && len(parts) != 4 {
		// Accept signed decimal degrees for small synthetic fixtures too.
		decimal, err := strconv.ParseFloat(body, 64)
		if err != nil || math.IsNaN(decimal) || math.IsInf(decimal, 0) {
			return 0, errors.New("expected GRPlugin DDD.MM.SS.sss or decimal degrees")
		}
		if direction == 'S' || direction == 'W' {
			decimal = -decimal
		}
		return decimal, nil
	}
	degrees, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, errors.New("invalid degrees")
	}
	minutes, err := strconv.ParseFloat(parts[1], 64)
	if err != nil || minutes < 0 || minutes >= 60 {
		return 0, errors.New("invalid minutes")
	}
	secondsText := parts[2]
	if len(parts) == 4 {
		secondsText += "." + parts[3]
	}
	seconds, err := strconv.ParseFloat(secondsText, 64)
	if err != nil || seconds < 0 || seconds >= 60 {
		return 0, errors.New("invalid seconds")
	}
	if degrees < 0 {
		return 0, errors.New("degrees must be non-negative")
	}
	coordinate := degrees + minutes/60 + seconds/3600
	if direction == 'S' || direction == 'W' {
		coordinate = -coordinate
	}
	return coordinate, nil
}

func normalizeToken(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func cloneStandCapability(capability StandCapability) StandCapability {
	capability.Blocks = slices.Clone(capability.Blocks)
	capability.WTC = slices.Clone(capability.WTC)
	capability.NotWTC = slices.Clone(capability.NotWTC)
	capability.EngineTypes = slices.Clone(capability.EngineTypes)
	capability.NotEngineTypes = slices.Clone(capability.NotEngineTypes)
	capability.AircraftTypes = slices.Clone(capability.AircraftTypes)
	capability.NotAircraftTypes = slices.Clone(capability.NotAircraftTypes)
	capability.Areas = slices.Clone(capability.Areas)
	return capability
}

func cloneStand(stand Stand) Stand {
	stand.Blocks = slices.Clone(stand.Blocks)
	variants := stand.Variants
	stand.Variants = make([]StandCapability, len(variants))
	for i, variant := range variants {
		stand.Variants[i] = cloneStandCapability(variant)
	}
	return stand
}
