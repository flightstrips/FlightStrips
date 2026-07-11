// Package sat contains the backend foundations for the Stand Assignment Tool.
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

// AircraftUseCode is the unmodified GRPlugin aircraft use code.
type AircraftUseCode string

const (
	AircraftUseCodeA AircraftUseCode = "A"
	AircraftUseCodeB AircraftUseCode = "B"
	AircraftUseCodeC AircraftUseCode = "C"
	AircraftUseCodeH AircraftUseCode = "H"
	AircraftUseCodeI AircraftUseCode = "I"
	AircraftUseCodeM AircraftUseCode = "M"
	AircraftUseCodeP AircraftUseCode = "P"
	AircraftUseCodeT AircraftUseCode = "T"
)

var validAircraftUseCodes = map[AircraftUseCode]struct{}{
	AircraftUseCodeA: {},
	AircraftUseCodeB: {},
	AircraftUseCodeC: {},
	AircraftUseCodeH: {},
	AircraftUseCodeI: {},
	AircraftUseCodeM: {},
	AircraftUseCodeP: {},
	AircraftUseCodeT: {},
}

// Aircraft contains the physical facts from GRpluginAircraftInfo.txt. Dimensions
// are metres and MTOW is kilograms, matching the source file.
type Aircraft struct {
	Type           string
	WingspanMetres float64
	LengthMetres   float64
	HeightMetres   float64
	MTOWKilograms  float64
	UseCode        AircraftUseCode
	Aliases        []string
}

// AircraftRegistry provides read-only lookup by canonical ICAO type or alias.
type AircraftRegistry struct {
	byType   map[string]Aircraft
	warnings []string
}

// Types returns all canonical aircraft types in deterministic order.
func (r *AircraftRegistry) Types() []string {
	if r == nil {
		return nil
	}

	types := make([]string, 0, len(r.byType))
	for aircraftType, facts := range r.byType {
		if aircraftType == facts.Type {
			types = append(types, aircraftType)
		}
	}
	slices.Sort(types)
	return types
}

// Lookup resolves an ICAO type or source alias, case-insensitively. The returned
// value is a copy, including its aliases, so callers cannot modify the registry.
func (r *AircraftRegistry) Lookup(aircraftType string) (Aircraft, bool) {
	if r == nil {
		return Aircraft{}, false
	}

	facts, ok := r.byType[normalizeAircraftToken(aircraftType)]
	if !ok {
		return Aircraft{}, false
	}
	facts.Aliases = append([]string(nil), facts.Aliases...)
	return facts, true
}

// Warnings reports non-fatal source ambiguities, including duplicate aliases.
// GRplugin uses several broad aliases for multiple specific aircraft records;
// those aliases resolve deterministically to their first declaration.
func (r *AircraftRegistry) Warnings() []string {
	if r == nil {
		return nil
	}
	return append([]string(nil), r.warnings...)
}

// LoadAircraftReference parses GRpluginAircraftInfo.txt data into a validated
// registry. Invalid rows are collected so a configuration error identifies every
// source line that needs correction.
func LoadAircraftReference(source io.Reader) (*AircraftRegistry, error) {
	if source == nil {
		return nil, errors.New("aircraft reference source is nil")
	}

	registry := &AircraftRegistry{byType: make(map[string]Aircraft)}
	canonicalLines := make(map[string]int)
	keyLines := make(map[string]int)
	var problems []error

	scanner := bufio.NewScanner(source)
	lineNumber := 0
	for scanner.Scan() {
		lineNumber++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		aircraft, err := parseAircraftRecord(line)
		if err != nil {
			problems = append(problems, fmt.Errorf("line %d: %w", lineNumber, err))
			continue
		}

		if firstLine, exists := canonicalLines[aircraft.Type]; exists {
			problems = append(problems, fmt.Errorf("line %d: duplicate canonical type %q (first declared on line %d)", lineNumber, aircraft.Type, firstLine))
			continue
		}
		canonicalLines[aircraft.Type] = lineNumber

		if firstLine, exists := keyLines[aircraft.Type]; exists {
			registry.warnings = append(registry.warnings, fmt.Sprintf("line %d: canonical type %q replaces alias declared on line %d", lineNumber, aircraft.Type, firstLine))
		}
		registry.byType[aircraft.Type] = aircraft
		keyLines[aircraft.Type] = lineNumber
		for _, alias := range aircraft.Aliases {
			if firstLine, exists := keyLines[alias]; exists {
				registry.warnings = append(registry.warnings, fmt.Sprintf("line %d: conflicting alias %q already declared on line %d; using the first declaration", lineNumber, alias, firstLine))
				continue
			}
			registry.byType[alias] = aircraft
			keyLines[alias] = lineNumber
		}
	}
	if err := scanner.Err(); err != nil {
		problems = append(problems, fmt.Errorf("read aircraft reference data: %w", err))
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid aircraft reference data: %w", errors.Join(problems...))
	}

	return registry, nil
}

// LoadAircraftReferenceFile opens and parses a committed aircraft reference file.
func LoadAircraftReferenceFile(path string) (*AircraftRegistry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open aircraft reference file %q: %w", path, err)
	}
	defer f.Close()

	registry, err := LoadAircraftReference(f)
	if err != nil {
		return nil, fmt.Errorf("load aircraft reference file %q: %w", path, err)
	}
	return registry, nil
}

func parseAircraftRecord(line string) (Aircraft, error) {
	columns := strings.Split(line, "\t")
	if len(columns) != 6 && len(columns) != 7 {
		return Aircraft{}, fmt.Errorf("malformed row: expected 6 or 7 tab-separated columns, got %d", len(columns))
	}

	canonicalType := normalizeAircraftToken(columns[0])
	if canonicalType == "" {
		return Aircraft{}, errors.New("canonical type is empty")
	}

	wingspan, err := parsePositiveNumber("wingspan", columns[1])
	if err != nil {
		return Aircraft{}, err
	}
	length, err := parsePositiveNumber("length", columns[2])
	if err != nil {
		return Aircraft{}, err
	}
	height, err := parsePositiveNumber("height", columns[3])
	if err != nil {
		return Aircraft{}, err
	}
	mtow, err := parsePositiveNumber("MTOW", columns[4])
	if err != nil {
		return Aircraft{}, err
	}

	useCode := AircraftUseCode(normalizeAircraftToken(columns[5]))
	if _, ok := validAircraftUseCodes[useCode]; !ok {
		return Aircraft{}, fmt.Errorf("unknown use code %q", strings.TrimSpace(columns[5]))
	}

	aliases := []string(nil)
	if len(columns) == 7 {
		aliases, err = parseAircraftAliases(columns[6], canonicalType)
		if err != nil {
			return Aircraft{}, err
		}
	}

	return Aircraft{
		Type:           canonicalType,
		WingspanMetres: wingspan,
		LengthMetres:   length,
		HeightMetres:   height,
		MTOWKilograms:  mtow,
		UseCode:        useCode,
		Aliases:        aliases,
	}, nil
}

func parsePositiveNumber(name, value string) (float64, error) {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
		return 0, fmt.Errorf("invalid %s %q", name, strings.TrimSpace(value))
	}
	return number, nil
}

func parseAircraftAliases(value, canonicalType string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}

	seen := make(map[string]struct{})
	aliases := make([]string, 0)
	for _, rawAlias := range strings.FieldsFunc(value, func(r rune) bool {
		return r == '/' || r == ',' || r == ';'
	}) {
		alias := normalizeAircraftToken(rawAlias)
		if alias == "" {
			return nil, fmt.Errorf("invalid empty alias in %q", value)
		}
		if alias == canonicalType {
			continue
		}
		if _, exists := seen[alias]; exists {
			continue
		}
		seen[alias] = struct{}{}
		aliases = append(aliases, alias)
	}
	return aliases, nil
}

func normalizeAircraftToken(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}
