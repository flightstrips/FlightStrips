package config

import (
	"FlightStrips/internal/aman/terminal"
	"FlightStrips/internal/gsx"
	"FlightStrips/internal/sat"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type AirportLoadOptions struct {
	RepositoryRoot string
	LookupEnv      func(string) (string, bool)
	// DeploymentServices preserves application flags for availability: deployment.
	DeploymentServices map[string]bool
	// Navigation/EFB and GSX can consume assets independently of AMAN/SAT.
	NavigationEnabled bool
	GSXEnabled        bool
}

func (d *AirportDefinition) Resource(field string) (string, bool) {
	path, ok := d.resources[field]
	return path, ok
}
func (d *AirportDefinition) ServiceEnabled(name string) bool { return d.services[name] }

// LoadAirportFiles opens only explicitly selected files; it never scans a
// directory. All definitions/resources validate before a registry is returned.
func LoadAirportFiles(catalog *AirportCatalog, selected []string, options AirportLoadOptions) (*AirportRegistry, error) {
	root, err := filepath.Abs(options.RepositoryRoot)
	if err != nil {
		return nil, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("repository root: %w", err)
	}
	if options.LookupEnv == nil {
		options.LookupEnv = os.LookupEnv
	}
	inputs := make([]AirportInput, 0, len(selected))
	for _, name := range selected {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("selected airport filename cannot be blank")
		}
		path := name
		if !filepath.IsAbs(path) {
			path = filepath.Join(root, path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("selected airport %s: %w", path, err)
		}
		inputs = append(inputs, AirportInput{Source: path, Reader: bytes.NewReader(data)})
	}
	registry, err := NewAirportRegistry(catalog, inputs)
	if err != nil {
		return nil, err
	}
	for _, airport := range registry.Airports() {
		definition, _ := registry.Lookup(airport)
		if err := definition.resolveResources(root, options); err != nil {
			return nil, fmt.Errorf("%s airport %s: %w", definition.source, airport, err)
		}
	}
	return registry, nil
}

func (d *AirportDefinition) resolveResources(root string, options AirportLoadOptions) error {
	d.resources = map[string]string{}
	d.services = map[string]bool{}
	hash := sha256.New()
	fmt.Fprintln(hash, d.revision)
	add := func(field string, resource *AirportResource) error {
		if resource == nil {
			return fmt.Errorf("%s: required resource is missing", field)
		}
		path, err := resolveAirportResource(root, *resource, options.LookupEnv)
		if err != nil {
			return fmt.Errorf("%s: %w", field, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%s selected resource %s: %w", field, path, err)
		}
		d.resources[field] = path
		fmt.Fprintf(hash, "%s\x00%s\x00%x\n", field, path, sha256.Sum256(data))
		return nil
	}
	if resource := d.document.Geography.Regions; resource != nil {
		if err := add("geography.regions", resource); err != nil {
			return err
		}
		data, err := os.ReadFile(d.resources["geography.regions"])
		if err != nil {
			return err
		}
		ground, runways, final, err := decodeRegions(bytes.NewReader(data))
		if err != nil {
			return fmt.Errorf("geography.regions: %w", err)
		}
		d.regionGroups = [3][]Region{ground, runways, final}
		names := map[string]bool{}
		for _, group := range [][]Region{ground, runways, final} {
			for _, region := range group {
				names[region.Name] = true
			}
		}
		for _, duty := range d.document.Coverage.Responsibilities {
			for _, region := range duty.Region {
				if !names[region] {
					return fmt.Errorf("coverage.responsibilities[%s].region: unknown %q in geography.regions", duty.ID, region)
				}
			}
		}
	}
	active := func(name string, availability Availability) bool {
		enabled := availability == ServiceEnabled || availability == ServiceDeployment && options.DeploymentServices[name]
		d.services[name] = enabled
		fmt.Fprintf(hash, "service:%s=%t\n", name, enabled)
		return enabled
	}
	services := d.document.Services
	if c := services.CDM; c != nil && active("cdm", c.Availability) {
		configRoot, err := constrainedRepositoryPath(root, c.ConfigRoot)
		if err != nil {
			return fmt.Errorf("services.cdm.config_root: %w", err)
		}
		d.resources["services.cdm.config_root"] = configRoot
		for _, item := range []struct{ field, uri string }{{"rateUri", c.Config.RateUri}, {"sidIntervalUri", c.Config.SidIntervalUri}, {"taxizonesUri", c.Config.TaxizonesUri}} {
			parsed, err := url.Parse(item.uri)
			if err != nil {
				return fmt.Errorf("services.cdm.config.%s: %w", item.field, err)
			}
			if parsed.Scheme == "http" || parsed.Scheme == "https" {
				if parsed.Host == "" {
					return fmt.Errorf("services.cdm.config.%s: missing URI host", item.field)
				}
				continue
			}
			if parsed.Scheme != "" || filepath.IsAbs(item.uri) || filepath.VolumeName(item.uri) != "" {
				return fmt.Errorf("services.cdm.config.%s: unsupported URI %q", item.field, item.uri)
			}
			selected, err := constrainedRepositoryPath(configRoot, item.uri)
			if err != nil {
				return fmt.Errorf("services.cdm.config.%s: %w", item.field, err)
			}
			relative, err := filepath.Rel(root, selected)
			if err != nil {
				return err
			}
			relative = filepath.ToSlash(relative)
			if err := add("services.cdm.config."+item.field, &AirportResource{RepoPath: relative}); err != nil {
				return err
			}
		}
	}
	if s := services.StandAssignment; s != nil && active("stand_assignment", s.Availability) {
		for _, item := range []struct {
			field    string
			resource *AirportResource
		}{{"stands", s.Stands}, {"aircraft_reference", s.AircraftReference}, {"aircraft_icao", s.AircraftICAO}, {"airline_assignment", s.AirlineAssignment}, {"scenery", s.Scenery}} {
			if err := add("services.stand_assignment."+item.field, item.resource); err != nil {
				return err
			}
		}
		resource := func(field string) string { return d.resources["services.stand_assignment."+field] }
		aircraft, err := sat.LoadAircraftReferenceFile(resource("aircraft_reference"))
		if err != nil {
			return fmt.Errorf("services.stand_assignment.aircraft_reference: %w", err)
		}
		if _, err := sat.LoadAircraftEngineReferenceFile(resource("aircraft_icao"), aircraft); err != nil {
			return fmt.Errorf("services.stand_assignment.aircraft_icao: %w", err)
		}
		stands, err := sat.LoadStandCapabilityFile(resource("stands"))
		if err != nil {
			return fmt.Errorf("services.stand_assignment.stands: %w", err)
		}
		if _, err := sat.LoadAirlineAssignmentFile(resource("airline_assignment"), stands); err != nil {
			return fmt.Errorf("services.stand_assignment.airline_assignment: %w", err)
		}
		if scenery, err := gsx.LoadSceneryConfig(resource("scenery")); err != nil {
			return fmt.Errorf("services.stand_assignment.scenery: %w", err)
		} else if scenery.ICAO != d.Airport() {
			return fmt.Errorf("services.stand_assignment.scenery: airport %s does not match %s", scenery.ICAO, d.Airport())
		}
	}
	if s := services.TextPilot; s != nil && active("text_pilot", s.Availability) {
		if err := add("services.text_pilot.layout_data", s.LayoutData); err != nil {
			return err
		}
	}
	if s := services.AMAN; s != nil && active("aman", s.Availability) {
		// AMAN needs shared aircraft facts even when SAT and its other assets
		// are disabled. Resolve only that dependency, using the same selection.
		if d.resources["services.stand_assignment.aircraft_icao"] == "" {
			if services.StandAssignment == nil {
				return fmt.Errorf("services.aman: services.stand_assignment.aircraft_icao resource is required")
			}
			if err := add("services.stand_assignment.aircraft_icao", services.StandAssignment.AircraftICAO); err != nil {
				return err
			}
			if _, err := sat.LoadAircraftEngineReferenceFile(d.resources["services.stand_assignment.aircraft_icao"], nil); err != nil {
				return fmt.Errorf("services.aman aircraft reference: %w", err)
			}
		}
		if err := add("services.aman.terminal", s.Terminal); err != nil {
			return err
		}
		if err := add("services.aman.boundary", s.Boundary); err != nil {
			return err
		}
		configuration, err := terminal.LoadFile(d.resources["services.aman.terminal"])
		if err != nil {
			return fmt.Errorf("services.aman.terminal: %w", err)
		}
		if string(configuration.Airport) != d.Airport() {
			return fmt.Errorf("services.aman.terminal: airport %s does not match %s", configuration.Airport, d.Airport())
		}
		if err := configuration.ValidateOperationalSettings(); err != nil {
			return fmt.Errorf("services.aman.terminal: %w", err)
		}
		if _, err := terminal.LoadTMAVolume(d.resources["services.aman.boundary"]); err != nil {
			return fmt.Errorf("services.aman.boundary: %w", err)
		}
	}
	if s := services.PDC; s != nil {
		active("pdc", s.Availability)
	}
	if s := services.ECFMP; s != nil {
		active("ecfmp", s.Availability)
	}
	if options.NavigationEnabled && d.resources["services.aman.terminal"] == "" {
		if services.AMAN == nil {
			return fmt.Errorf("navigation: services.aman.terminal must be configured")
		}
		if err := add("services.aman.terminal", services.AMAN.Terminal); err != nil {
			return err
		}
		configuration, err := terminal.LoadFile(d.resources["services.aman.terminal"])
		if err != nil {
			return fmt.Errorf("navigation terminal: %w", err)
		}
		if string(configuration.Airport) != d.Airport() {
			return fmt.Errorf("navigation terminal airport %s does not match %s", configuration.Airport, d.Airport())
		}
	}
	if options.GSXEnabled && d.resources["services.stand_assignment.scenery"] == "" {
		if services.StandAssignment == nil {
			return fmt.Errorf("GSX: services.stand_assignment.scenery must be configured")
		}
		if err := add("services.stand_assignment.scenery", services.StandAssignment.Scenery); err != nil {
			return err
		}
		scenery, err := gsx.LoadSceneryConfig(d.resources["services.stand_assignment.scenery"])
		if err != nil {
			return fmt.Errorf("services.stand_assignment.scenery: %w", err)
		}
		if scenery == nil || scenery.ICAO != d.Airport() {
			return fmt.Errorf("services.stand_assignment.scenery: must describe airport %s", d.Airport())
		}
	}
	d.revision = fmt.Sprintf("%x", hash.Sum(nil))
	return nil
}

func resolveAirportResource(root string, resource AirportResource, lookup func(string) (string, bool)) (string, error) {
	if resource.RepoPath != "" {
		return constrainedRepositoryPath(root, resource.RepoPath)
	}
	// Validate the default even when an external deployment override is selected.
	fallback, err := constrainedRepositoryPath(root, resource.DefaultRepoPath)
	if err != nil {
		return "", err
	}
	override, _ := lookup(resource.Environment)
	override = strings.TrimSpace(override)
	if override == "" {
		return fallback, nil
	}
	if !filepath.IsAbs(override) {
		override = filepath.Join(root, override)
	}
	return filepath.Abs(override)
}

func constrainedRepositoryPath(root, relative string) (string, error) {
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", fmt.Errorf("resource root %s: %w", root, err)
	}
	root = canonicalRoot
	if relative == "" || filepath.IsAbs(relative) || filepath.VolumeName(relative) != "" || strings.Contains(relative, "\\") || slices.Contains(strings.Split(relative, "/"), "..") {
		return "", fmt.Errorf("repository path escapes or is invalid: %q", relative)
	}
	result := filepath.Join(root, filepath.FromSlash(relative))
	candidate := result
	// Check the nearest existing ancestor as well, so dangling/missing children
	// under an outward symlink cannot bypass the repository boundary.
	for {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err == nil {
			within, err := filepath.Rel(root, resolved)
			if err != nil || within == ".." || strings.HasPrefix(within, ".."+string(filepath.Separator)) {
				return "", fmt.Errorf("repository path escapes through symlink: %q", relative)
			}
			break
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(candidate)
		if parent == candidate {
			return "", err
		}
		candidate = parent
	}
	return result, nil
}
