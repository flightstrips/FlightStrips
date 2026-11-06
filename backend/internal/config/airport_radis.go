package config

import "fmt"

// RADISSettings contains explicit activation inputs. No prototype/ENGM operating
// constants are defaults. One compiled implementation serves independent boards.
type RADISSettings struct {
	DepartureInitialBay     string                    `yaml:"departure_initial_bay"`
	ArrivalInitialBay       string                    `yaml:"arrival_initial_bay"`
	BayGroundStates         map[string]RADISBayStates `yaml:"bay_ground_states"`
	AdmissionDistanceNM     float64                   `yaml:"admission_distance_nm"`
	DepartureAdmissionAGLFT int32                     `yaml:"departure_admission_agl_ft"`
	RemovalDistanceNM       float64                   `yaml:"removal_distance_nm"`
	DepartureRemovalAGLFT   int32                     `yaml:"departure_removal_agl_ft"`
	RecoverySeconds         int                       `yaml:"recovery_seconds"`
	TimestampSemantics      string                    `yaml:"timestamp_semantics"`
}

type RADISBayStates struct {
	Departure string `yaml:"departure"`
	Arrival   string `yaml:"arrival"`
}

var RADISBays = []string{"ACTIVE", "RUNWAY", "PASSIVE_TOP", "PASSIVE_BOTTOM"}

func RADISDescriptor() HandlerDescriptor {
	return HandlerDescriptor{ID: "radis/v1", View: "radis", Layouts: []string{"RADIS"}, RequiredBays: RADISBays,
		DecodeSettings: TypedAirportSettings[RADISSettings], Validate: func(d *AirportDocument) error {
			s, ok := d.Workflow.Settings.(RADISSettings)
			if !ok {
				return fmt.Errorf("workflow.settings: expected RADISSettings")
			}
			if d.Ownership.Strips != "none" || d.Ownership.Sectors != "none" {
				return fmt.Errorf("RADIS requires ownership disabled")
			}
			if len(d.Workflow.Bays) != len(RADISBays) {
				return fmt.Errorf("RADIS requires exactly four bays")
			}
			known := func(value string) bool {
				for _, b := range RADISBays {
					if b == value {
						return true
					}
				}
				return false
			}
			if !known(s.DepartureInitialBay) || !known(s.ArrivalInitialBay) {
				return fmt.Errorf("unknown RADIS initial bay")
			}
			if len(s.BayGroundStates) != 4 {
				return fmt.Errorf("RADIS requires explicit bay mappings")
			}
			for b := range s.BayGroundStates {
				if !known(b) {
					return fmt.Errorf("unknown RADIS bay mapping %s", b)
				}
			}
			if s.RemovalDistanceNM < s.AdmissionDistanceNM || s.DepartureRemovalAGLFT <= s.DepartureAdmissionAGLFT {
				return fmt.Errorf("RADIS removal bounds must exceed admission bounds")
			}
			if d.Services.PDC != nil && d.Services.PDC.Availability != ServiceDisabled || d.Services.AMAN != nil && d.Services.AMAN.Availability != ServiceDisabled {
				return fmt.Errorf("RADIS PDC and AMAN must be disabled")
			}
			return nil
		}}
}
