package sat

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// EngineType is the EuroScope/GRPlugin engine code used by stand capability
// rules. The zero value is never a resolved engine type.
type EngineType string

const (
	EngineUnknown   EngineType = "UNKNOWN"
	EnginePiston    EngineType = "P"
	EngineTurboprop EngineType = "T"
	EngineJet       EngineType = "J"
	EngineElectric  EngineType = "E"
)

func parseEngineType(value string) (EngineType, bool) {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case string(EnginePiston):
		return EnginePiston, true
	case string(EngineTurboprop):
		return EngineTurboprop, true
	case string(EngineJet):
		return EngineJet, true
	case string(EngineElectric):
		return EngineElectric, true
	default:
		return EngineUnknown, false
	}
}

// ParseEngineType validates a live EuroScope engine value.
func ParseEngineType(value string) (EngineType, bool) {
	return parseEngineType(value)
}

// AircraftEngineFacts contains engine and WTC facts from the installed ICAO
// aircraft database.
type AircraftEngineFacts struct {
	EngineType EngineType
	WTC        string
}

// AircraftEngineRegistry is a read-only view of the installed ICAO aircraft
// database. It contains no copied database rows or generated mapping data.
type AircraftEngineRegistry struct {
	byType   map[string]AircraftEngineFacts
	aircraft *AircraftRegistry
}

type icaoAircraftRecord struct {
	ICAO        string `json:"ICAO"`
	Description string `json:"Description"`
	WTC         string `json:"WTC"`
	IATA        string `json:"IATA"`
	IATACargo   string `json:"IATA_cargo"`
}

// Lookup resolves an ICAO type or an ICAO JSON record's IATA alias.
func (r *AircraftEngineRegistry) Lookup(aircraftType string) (EngineType, bool) {
	facts, ok := r.lookup(aircraftType)
	if !ok || facts.EngineType == EngineUnknown {
		return EngineUnknown, false
	}
	return facts.EngineType, true
}

// LookupWTC resolves the WTC published by the installed aircraft database.
func (r *AircraftEngineRegistry) LookupWTC(aircraftType string) (string, bool) {
	facts, ok := r.lookup(aircraftType)
	if !ok || facts.WTC == "UNKNOWN" {
		return "", false
	}
	return facts.WTC, true
}

func (r *AircraftEngineRegistry) lookup(aircraftType string) (AircraftEngineFacts, bool) {
	if r == nil {
		return AircraftEngineFacts{}, false
	}
	key := normalizeAircraftToken(aircraftType)
	if facts, ok := r.byType[key]; ok {
		return facts, true
	}
	if r.aircraft != nil {
		if reference, ok := r.aircraft.Lookup(key); ok {
			facts, ok := r.byType[reference.Type]
			return facts, ok
		}
	}
	return AircraftEngineFacts{}, false
}

// LoadAircraftEngineReference parses the installed ICAO_Aircraft.json data.
func LoadAircraftEngineReference(source io.Reader, aircraft *AircraftRegistry) (*AircraftEngineRegistry, error) {
	if source == nil {
		return nil, errors.New("ICAO aircraft source is nil")
	}

	data, err := io.ReadAll(source)
	if err != nil {
		return nil, fmt.Errorf("read ICAO aircraft JSON: %w", err)
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	var records []icaoAircraftRecord
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&records); err != nil {
		return nil, fmt.Errorf("decode ICAO aircraft JSON: %w", err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return nil, errors.New("ICAO aircraft JSON contains multiple top-level values")
		}
		return nil, fmt.Errorf("read ICAO aircraft JSON tail: %w", err)
	}

	registry := &AircraftEngineRegistry{byType: make(map[string]AircraftEngineFacts, len(records)), aircraft: aircraft}
	canonicalFacts := make(map[string]AircraftEngineFacts, len(records))
	type aliasEntry struct {
		alias string
		facts AircraftEngineFacts
	}
	var aliases []aliasEntry
	var problems []error
	for index, record := range records {
		recordNumber := index + 1
		aircraftType := normalizeAircraftToken(record.ICAO)
		if aircraftType == "" {
			problems = append(problems, fmt.Errorf("record %d: ICAO type is required", recordNumber))
			continue
		}
		engine, valid := parseDescriptionEngine(record.Description)
		if !valid {
			problems = append(problems, fmt.Errorf("record %d (%s): invalid engine code in Description %q", recordNumber, aircraftType, record.Description))
			continue
		}
		wtc := "UNKNOWN"
		if strings.TrimSpace(record.WTC) != "" {
			wtc = strings.ToUpper(strings.TrimSpace(record.WTC))
			if !validWTC(wtc) {
				problems = append(problems, fmt.Errorf("record %d (%s): invalid WTC %q", recordNumber, aircraftType, record.WTC))
				continue
			}
		}
		if _, duplicate := canonicalFacts[aircraftType]; duplicate {
			problems = append(problems, fmt.Errorf("record %d: duplicate ICAO type %q", recordNumber, aircraftType))
			continue
		}
		facts := AircraftEngineFacts{EngineType: engine, WTC: wtc}
		canonicalFacts[aircraftType] = facts
		for _, alias := range []string{record.IATA, record.IATACargo} {
			alias = normalizeAircraftToken(alias)
			if alias == "" || alias == aircraftType {
				continue
			}
			aliases = append(aliases, aliasEntry{alias: alias, facts: facts})
		}
	}
	if len(problems) > 0 {
		return nil, fmt.Errorf("invalid ICAO aircraft data: %w", errors.Join(problems...))
	}
	for aircraftType, facts := range canonicalFacts {
		registry.byType[aircraftType] = facts
	}
	for _, entry := range aliases {
		if _, canonical := registry.byType[entry.alias]; canonical {
			continue
		}
		if _, exists := registry.byType[entry.alias]; !exists {
			registry.byType[entry.alias] = entry.facts
		}
	}
	return registry, nil
}

func parseDescriptionEngine(description string) (EngineType, bool) {
	description = strings.TrimSpace(description)
	if description == "" || strings.HasSuffix(description, "-") || strings.HasSuffix(description, "R") {
		return EngineUnknown, true
	}
	return parseEngineType(description[len(description)-1:])
}

func validWTC(value string) bool {
	switch value {
	case "L", "M", "H", "J":
		return true
	default:
		return false
	}
}

// LoadAircraftEngineReferenceFile opens the installed ICAO aircraft database.
func LoadAircraftEngineReferenceFile(path string, aircraft *AircraftRegistry) (*AircraftEngineRegistry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ICAO aircraft file %q: %w", path, err)
	}
	defer f.Close()

	registry, err := LoadAircraftEngineReference(f, aircraft)
	if err != nil {
		return nil, fmt.Errorf("load ICAO aircraft file %q: %w", path, err)
	}
	return registry, nil
}
