package config

// AirportDocument is the fully decoded, versioned authoring contract. Loading it
// never installs configuration into the legacy EKCH consumers.
type AirportDocument struct {
	Schema        string              `yaml:"$schema,omitempty"`
	SchemaVersion string              `yaml:"schema_version"`
	Kind          string              `yaml:"kind"`
	Airport       string              `yaml:"airport"`
	Geography     AirportGeography    `yaml:"geography"`
	Positions     []Position          `yaml:"positions"`
	Coverage      AirportCoverage     `yaml:"coverage"`
	Master        AirportMaster       `yaml:"master"`
	Ownership     AirportOwnership    `yaml:"ownership"`
	Routing       AirportRouting      `yaml:"routing"`
	View          AirportView         `yaml:"view"`
	Workflow      AirportWorkflow     `yaml:"workflow"`
	Services      AirportServices     `yaml:"services"`
	Validation    AirportValidation   `yaml:"validation"`
	Provenance    []AirportProvenance `yaml:"provenance"`
}

type AirportResource struct {
	RepoPath        string `yaml:"repo_path,omitempty"`
	Environment     string `yaml:"environment,omitempty"`
	DefaultRepoPath string `yaml:"default_repo_path,omitempty"`
}
type AirportGeography struct {
	Latitude             float64          `yaml:"latitude"`
	Longitude            float64          `yaml:"longitude"`
	ElevationFT          float64          `yaml:"elevation_ft"`
	TransitionAltitudeFT int              `yaml:"transition_altitude_ft"`
	Runways              []string         `yaml:"runways"`
	InitialCFLFT         map[string]int   `yaml:"initial_cfl_ft"`
	Regions              *AirportResource `yaml:"regions,omitempty"`
}
type AirportCoverage struct {
	Matching         CoverageMatching        `yaml:"matching"`
	Responsibilities []AirportResponsibility `yaml:"responsibilities"`
	AirborneFallback []string                `yaml:"airborne_fallback"`
	Membership       CoverageMembership      `yaml:"membership"`
}
type CoverageMatching struct {
	PrimaryFrequency  bool   `yaml:"primary_frequency"`
	TunedFrequencies  bool   `yaml:"tuned_frequencies"`
	CallsignFallback  bool   `yaml:"callsign_fallback"`
	Normalization     string `yaml:"normalization"`
	ProviderSelection string `yaml:"provider_selection"`
}
type CoverageMembership struct {
	Policy                 string   `yaml:"policy"`
	RelinquishGraceSeconds *float64 `yaml:"relinquish_grace_seconds,omitempty"`
}
type AirportResponsibility struct {
	ID           string       `yaml:"id"`
	Name         string       `yaml:"name"`
	Key          string       `yaml:"key,omitempty"`
	DisplayName  string       `yaml:"display_name,omitempty"`
	NamePriority int          `yaml:"name_priority,omitempty"`
	Region       []string     `yaml:"region,omitempty"`
	Constraints  *constraints `yaml:"constraints,omitempty"`
	Active       []string     `yaml:"active,omitempty"`
	RequireAll   bool         `yaml:"require_all,omitempty"`
	Providers    []string     `yaml:"providers"`
}
type AirportMaster struct {
	Priorities            []MasterPriority `yaml:"priorities"`
	PreemptHigherPriority bool             `yaml:"preempt_higher_priority"`
	EqualPriority         string           `yaml:"equal_priority"`
	Fallback              string           `yaml:"fallback"`
	ObserversEligible     bool             `yaml:"observers_eligible"`
}
type MasterPriority struct {
	Position string `yaml:"position"`
	Match    string `yaml:"match"`
}
type AirportOwnership struct {
	Strips           string   `yaml:"strips"`
	Sectors          string   `yaml:"sectors"`
	CallsignPrefixes []string `yaml:"callsign_prefixes"`
}
type AirportRouting struct {
	Ground         []AirportRoute    `yaml:"ground"`
	Airborne       []AirborneRoutes  `yaml:"airborne"`
	MissedApproach map[string]string `yaml:"missed_approach"`
}
type AirportRoute struct {
	ID    string `yaml:"id"`
	Route `yaml:",inline"`
}
type AirportView struct {
	ID                     string                     `yaml:"id"`
	Layouts                map[string][]LayoutVariant `yaml:"layouts"`
	AirborneFallbackLayout string                     `yaml:"airborne_fallback_layout"`
	MessageAreas           map[string][]string        `yaml:"message_areas"`
}
type AirportWorkflow struct {
	Handler string `yaml:"handler"`
	// Settings is replaced with the concrete compiled settings type before DecodeAirport returns.
	Settings    any                          `yaml:"settings"`
	Bays        []AirportBay                 `yaml:"bays"`
	Annotations map[string]AirportAnnotation `yaml:"annotations,omitempty"`
}
type EKCHSettings struct {
	AirborneAltitudeAGLFT int64   `yaml:"airborne_altitude_agl_ft"`
	RelevantDistanceNM    float64 `yaml:"relevant_distance_nm"`
}
type AirportBay struct {
	ID    string `yaml:"id"`
	Label string `yaml:"label"`
}
type AirportAnnotation struct {
	Type    string `yaml:"type"`
	Scope   string `yaml:"scope"`
	Default bool   `yaml:"default"`
}
type Availability string

const (
	ServiceEnabled    Availability = "enabled"
	ServiceDisabled   Availability = "disabled"
	ServiceDeployment Availability = "deployment"
)

type AirportServices struct {
	CDM             *AirportCDM       `yaml:"cdm,omitempty"`
	StandAssignment *AirportStands    `yaml:"stand_assignment,omitempty"`
	TextPilot       *AirportTextPilot `yaml:"text_pilot,omitempty"`
	AMAN            *AirportAMAN      `yaml:"aman,omitempty"`
	PDC             *AirportPDC       `yaml:"pdc,omitempty"`
	ECFMP           *AirportService   `yaml:"ecfmp,omitempty"`
}
type AirportService struct {
	Availability Availability `yaml:"availability"`
}
type AirportCDM struct {
	AirportService `yaml:",inline"`
	ConfigRoot     string     `yaml:"config_root,omitempty"`
	Config         *CdmConfig `yaml:"config,omitempty"`
}
type AirportStands struct {
	AirportService    `yaml:",inline"`
	Stands            *AirportResource `yaml:"stands,omitempty"`
	AircraftReference *AirportResource `yaml:"aircraft_reference,omitempty"`
	AircraftICAO      *AirportResource `yaml:"aircraft_icao,omitempty"`
	AirlineAssignment *AirportResource `yaml:"airline_assignment,omitempty"`
	Scenery           *AirportResource `yaml:"scenery,omitempty"`
}
type AirportTextPilot struct {
	AirportService `yaml:",inline"`
	Provider       string           `yaml:"provider,omitempty"`
	LayoutData     *AirportResource `yaml:"layout_data,omitempty"`
}
type AirportAMAN struct {
	AirportService `yaml:",inline"`
	Terminal       *AirportResource `yaml:"terminal,omitempty"`
	Boundary       *AirportResource `yaml:"boundary,omitempty"`
}
type AirportPDC struct {
	AirportService `yaml:",inline"`
	Validation     *PDCValidationConfig `yaml:"validation,omitempty"`
}
type AirportValidation struct {
	Clearance    *ClxValidationConfig         `yaml:"clearance,omitempty"`
	TaxiwayTypes *TaxiwayTypeValidationConfig `yaml:"taxiway_types,omitempty"`
}
type AirportProvenance struct {
	Path   string `yaml:"path"`
	SHA256 string `yaml:"sha256"`
}
