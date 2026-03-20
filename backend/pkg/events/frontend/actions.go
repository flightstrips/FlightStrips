package frontend

// Action type constants — messages sent from the frontend to the backend.
const (
	ActionCreateManualFPL      EventType = "create_manual_fpl"
	ActionCreateVFRFPL         EventType = "create_vfr_fpl"
	ActionCreateTacticalStrip  EventType = "create_tactical_strip"
	ActionDeleteTacticalStrip  EventType = "delete_tactical_strip"
	ActionConfirmTacticalStrip EventType = "confirm_tactical_strip"
	ActionStartTacticalTimer   EventType = "start_tactical_timer"
	ActionMoveTacticalStrip    EventType = "move_tactical_strip"
)

// ---------- Manual FPL action payloads ----------

// CreateManualFPLAction is sent by the frontend to create an IFR flight plan for a connected aircraft.
type CreateManualFPLAction struct {
	Type         EventType `json:"type"`
	Callsign     string    `json:"callsign"`
	ADES         string    `json:"ades"`
	SID          string    `json:"sid"`
	SSR          string    `json:"ssr"`
	EOBT         string    `json:"eobt"`
	AircraftType string    `json:"aircraft_type"`
	FL           string    `json:"fl"`
	Route        string    `json:"route"`
	Stand        string    `json:"stand"`
	RwyDep       string    `json:"rwy_dep"`
}

// CreateVFRFPLAction is sent by the frontend to create a VFR flight plan for a connected aircraft.
type CreateVFRFPLAction struct {
	Type           EventType `json:"type"`
	Callsign       string    `json:"callsign"`
	AircraftType   string    `json:"aircraft_type"`
	PersonsOnBoard int       `json:"persons_on_board"`
	SSR            string    `json:"ssr"`
	FPLType        string    `json:"fpl_type"`
	Language       string    `json:"language"`
	Remarks        string    `json:"remarks"`
}

// ---------- Tactical strip action payloads ----------

type CreateTacticalStripAction struct {
	Type     EventType `json:"type"`
	StripType string   `json:"strip_type"` // "MEMAID" | "CROSSING" | "START" | "LAND"
	Bay      string    `json:"bay"`
	Label    string    `json:"label"`
	Aircraft string    `json:"aircraft"`
}

type DeleteTacticalStripAction struct {
	Type EventType `json:"type"`
	ID   int64     `json:"id"`
}

type ConfirmTacticalStripAction struct {
	Type EventType `json:"type"`
	ID   int64     `json:"id"`
}

type StartTacticalTimerAction struct {
	Type EventType `json:"type"`
	ID   int64     `json:"id"`
}

// MoveTacticalStripAction moves a tactical strip within a bay.
// InsertAfter is the strip immediately above the drop point (nil = move to top).
type MoveTacticalStripAction struct {
	Type        EventType `json:"type"`
	ID          int64     `json:"id"`
	InsertAfter *StripRef `json:"insert_after"`
}
