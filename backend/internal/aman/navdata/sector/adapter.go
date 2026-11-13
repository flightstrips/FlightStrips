// Package sector reads local EuroScope SCT navigation geometry. Drawing layers
// are not treated as procedures: AMAN's versioned terminal configuration remains
// responsible for STAR continuations, approaches, runways and holdings.
package sector

import (
	"bufio"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"FlightStrips/internal/aman"
	"FlightStrips/internal/aman/navdata"
	"FlightStrips/internal/aman/terminal"
)

const ResolverVersion = "sector-route-v2"

var cyclePattern = regexp.MustCompile(`(?i)\bAIRAC\s+(\d{4})\b`)
var infoCyclePattern = regexp.MustCompile(`\b(\d{4})/\d+`)

type point struct {
	id       navdata.FixID
	position navdata.Coordinate
}
type segment struct{ from, to navdata.Coordinate }

type Adapter struct {
	version    navdata.DatasetVersion
	provenance navdata.Provenance
	terminal   terminal.Configuration
	fixes      map[navdata.FixID]navdata.Coordinate
	ambiguous  map[navdata.FixID]bool
	airports   map[navdata.AirportID]navdata.Coordinate
	airways    map[string][]segment
	now        func() time.Time
}

func Load(path string, config terminal.Configuration, now func() time.Time) (*Adapter, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open sector navigation file: %w", err)
	}
	defer f.Close()
	return Read(f, config, now)
}

// Read uses only navigation sections. Comments and map annotations are data,
// never configuration or executable instructions. File content is fingerprinted
// so replacing an SCT cannot reuse routes from a different local revision.
func Read(r io.Reader, config terminal.Configuration, now func() time.Time) (*Adapter, error) {
	if now == nil {
		now = time.Now
	}
	a := &Adapter{terminal: config, fixes: map[navdata.FixID]navdata.Coordinate{}, ambiguous: map[navdata.FixID]bool{}, airports: map[navdata.AirportID]navdata.Coordinate{}, airways: map[string][]segment{}, now: now}
	hash := sha256.New()
	limited := &io.LimitedReader{R: r, N: (64 << 20) + 1}
	scanner := bufio.NewScanner(io.TeeReader(limited, hash))
	scanner.Buffer(make([]byte, 4096), 1<<20)
	section, cycle := "", ""
	lineNumber := 0
	type airwayRecord struct {
		name, fromX, fromY, toX, toY string
		line                         int
	}
	var airwayRecords []airwayRecord
	for scanner.Scan() {
		lineNumber++
		raw := strings.TrimPrefix(scanner.Text(), "\ufeff")
		if match := cyclePattern.FindStringSubmatch(raw); match != nil {
			if cycle != "" && cycle != match[1] {
				return nil, fmt.Errorf("sector file has conflicting AIRAC cycles")
			}
			cycle = match[1]
		}
		line := strings.TrimSpace(strings.SplitN(raw, ";", 2)[0])
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.ToUpper(line)
			continue
		}
		if section == "[INFO]" && cycle == "" {
			if match := infoCyclePattern.FindStringSubmatch(line); match != nil {
				cycle = match[1]
			}
		}
		fields := strings.Fields(strings.ToUpper(line))
		fail := func(err error) (*Adapter, error) {
			return nil, fmt.Errorf("sector %s line %d: %w", section, lineNumber, err)
		}
		switch section {
		case "[FIXES]", "[VOR]", "[NDB]", "[AIRPORT]":
			offset := 1
			if section != "[FIXES]" {
				offset = 2
			} else {
				offset = len(fields) - 2
			}
			if offset < 1 {
				return fail(fmt.Errorf("incomplete navigation record"))
			}
			if len(fields) < offset+2 {
				return fail(fmt.Errorf("incomplete navigation record"))
			}
			position, err := coordinate(fields[offset], fields[offset+1])
			if err != nil {
				return fail(err)
			}
			// Some SCTs include multi-word visual reporting labels in FIXES.
			// They are not ITEM15 identifiers; do not truncate them into a fix.
			if section == "[FIXES]" && offset != 1 {
				continue
			}
			id := navdata.FixID(fields[0])
			if section == "[AIRPORT]" {
				key := navdata.AirportID(id)
				if old, ok := a.airports[key]; ok && navdata.CoordinateDistanceNM(old, position) > 0.05 {
					return fail(fmt.Errorf("conflicting airport %s", id))
				}
				a.airports[key] = position
			} else {
				a.addFix(id, position)
			}
		case "[HIGH AIRWAY]", "[LOW AIRWAY]":
			if len(fields) < 5 {
				return fail(fmt.Errorf("incomplete airway segment"))
			}
			airwayRecords = append(airwayRecords, airwayRecord{fields[0], fields[1], fields[2], fields[3], fields[4], lineNumber})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read sector file: %w", err)
	}
	if limited.N == 0 {
		return nil, fmt.Errorf("sector file exceeds 64 MiB")
	}
	for _, record := range airwayRecords {
		from, err := a.airwayCoordinate(record.fromX, record.fromY)
		if err != nil {
			return nil, fmt.Errorf("sector airway line %d: %w", record.line, err)
		}
		to, err := a.airwayCoordinate(record.toX, record.toY)
		if err != nil {
			return nil, fmt.Errorf("sector airway line %d: %w", record.line, err)
		}
		a.airways[record.name] = append(a.airways[record.name], segment{from, to})
	}
	if cycle == "" || cycle != config.Dataset.Cycle {
		return nil, fmt.Errorf("sector AIRAC %q does not match terminal cycle %q", cycle, config.Dataset.Cycle)
	}
	a.version = navdata.DatasetVersion{Cycle: cycle, SourceRevision: fmt.Sprintf("sector-%x", hash.Sum(nil)), EffectiveFrom: config.Dataset.EffectiveFrom, EffectiveUntil: config.Dataset.EffectiveUntil}
	if err := a.version.Validate(); err != nil {
		return nil, err
	}
	a.provenance = navdata.Provenance{SourceID: "sector", SourceRevision: a.version.SourceRevision, ImportedAt: now().UTC().Truncate(time.Microsecond), EffectiveFrom: a.version.EffectiveFrom, EffectiveUntil: a.version.EffectiveUntil}
	if len(a.fixes) == 0 || len(a.airports) == 0 {
		return nil, fmt.Errorf("sector file has no usable fixes or airports")
	}
	return a, nil
}

func (a *Adapter) airwayCoordinate(x, y string) (navdata.Coordinate, error) {
	// SCT also permits named endpoints written twice (FIX FIX).
	if x == y {
		if position, ok := a.fixes[navdata.FixID(x)]; ok && !a.ambiguous[navdata.FixID(x)] {
			return position, nil
		}
		return navdata.Coordinate{}, incomplete("airway endpoint is missing or ambiguous: " + x)
	}
	return coordinate(x, y)
}

func (a *Adapter) addFix(id navdata.FixID, position navdata.Coordinate) {
	if old, ok := a.fixes[id]; ok && navdata.CoordinateDistanceNM(old, position) > 0.05 {
		a.ambiguous[id] = true
	}
	a.fixes[id] = position
}

func coordinate(lat, lon string) (navdata.Coordinate, error) {
	x, err := angle(lat, true)
	if err != nil {
		return navdata.Coordinate{}, err
	}
	y, err := angle(lon, false)
	if err != nil {
		return navdata.Coordinate{}, err
	}
	p := navdata.Coordinate{LatitudeDeg: x, LongitudeDeg: y}
	if err := p.Validate(); err != nil {
		return p, err
	}
	return p, nil
}

func angle(value string, latitude bool) (float64, error) {
	bad := func() (float64, error) { return 0, fmt.Errorf("invalid SCT coordinate %q", value) }
	if len(value) < 2 {
		return bad()
	}
	hemisphere := value[0]
	if latitude && hemisphere != 'N' && hemisphere != 'S' || !latitude && hemisphere != 'E' && hemisphere != 'W' {
		return bad()
	}
	parts := strings.SplitN(value[1:], ".", 3)
	if len(parts) != 3 {
		return bad()
	}
	degrees, e1 := strconv.Atoi(parts[0])
	minutes, e2 := strconv.Atoi(parts[1])
	seconds, e3 := strconv.ParseFloat(parts[2], 64)
	limit := 180
	if latitude {
		limit = 90
	}
	if e1 != nil || e2 != nil || e3 != nil || degrees < 0 || degrees > limit || minutes < 0 || minutes >= 60 || seconds < 0 || seconds >= 60 {
		return bad()
	}
	result := float64(degrees) + float64(minutes)/60 + seconds/3600
	if result > float64(limit) {
		return bad()
	}
	if hemisphere == 'S' || hemisphere == 'W' {
		result = -result
	}
	return result, nil
}

func (a *Adapter) compatible(ctx context.Context, version navdata.DatasetVersion) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := version.Validate(); err != nil {
		return err
	}
	if version.Cycle != a.version.Cycle || !version.EffectiveFrom.Equal(a.version.EffectiveFrom) || !version.EffectiveUntil.Equal(a.version.EffectiveUntil) {
		return &aman.DomainError{Class: navdata.ErrorDatasetMismatch, Message: "sector and active terminal datasets differ"}
	}
	if now := a.now().UTC(); now.Before(a.version.EffectiveFrom) || !now.Before(a.version.EffectiveUntil) {
		return &aman.DomainError{Class: navdata.ErrorDatasetMismatch, Message: "sector dataset is not currently effective"}
	}
	return nil
}

func (a *Adapter) LatestVersion(ctx context.Context) (navdata.DatasetVersion, error) {
	return a.version, a.compatible(ctx, a.version)
}
func (a *Adapter) Airport(ctx context.Context, v navdata.DatasetVersion, id navdata.AirportID) (navdata.Airport, error) {
	if err := a.compatible(ctx, v); err != nil {
		return navdata.Airport{}, err
	}
	p, ok := a.airports[id]
	if !ok {
		return navdata.Airport{}, incomplete("airport is absent from sector: " + string(id))
	}
	return navdata.Airport{ID: id, Name: string(id), Position: p, Provenance: a.provenance}, nil
}
func (a *Adapter) Fixes(ctx context.Context, q navdata.FixQuery) (navdata.FixSet, error) {
	if err := q.Validate(); err != nil {
		return navdata.FixSet{}, err
	}
	if err := a.compatible(ctx, q.Version); err != nil {
		return navdata.FixSet{}, err
	}
	set := navdata.FixSet{Version: q.Version, Coverage: navdata.CoverageComplete, Provenance: a.provenance}
	for _, id := range q.Identifiers {
		p, ok := a.fixes[id]
		if !ok || a.ambiguous[id] {
			return navdata.FixSet{}, incomplete("sector fix missing or ambiguous: " + string(id))
		}
		set.Fixes = append(set.Fixes, navdata.Fix{ID: id, Position: p, Provenance: a.provenance})
	}
	return set, set.Validate()
}
func (a *Adapter) Procedures(ctx context.Context, q navdata.ProcedureQuery) (navdata.ProcedureSet, error) {
	if err := q.Validate(); err != nil {
		return navdata.ProcedureSet{}, err
	}
	if err := a.compatible(ctx, q.Version); err != nil {
		return navdata.ProcedureSet{}, err
	}
	// SCT SID/STAR sections are display polylines, not ARINC procedure records.
	return navdata.ProcedureSet{Version: q.Version, Airport: q.Airport, Coverage: navdata.CoveragePartial, Provenance: a.provenance}, nil
}

func incomplete(message string) error {
	return &aman.DomainError{Class: navdata.ErrorIncompleteGeometry, Message: message}
}

// FixCount is useful when validating a locally supplied sector file.
func (a *Adapter) FixCount() int { return len(a.fixes) }

// HasFix distinguishes local waypoint identifiers from airway-like labels
// (for example CH626) while checking an online path against ITEM15.
func (a *Adapter) HasFix(id navdata.FixID) bool {
	_, exists := a.fixes[id]
	return exists && !a.ambiguous[id]
}
