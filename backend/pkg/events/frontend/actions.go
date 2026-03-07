package frontend

// Action type constants — messages sent from the frontend to the backend.
const (
	ActionCreateTacticalStrip  EventType = "create_tactical_strip"
	ActionDeleteTacticalStrip  EventType = "delete_tactical_strip"
	ActionConfirmTacticalStrip EventType = "confirm_tactical_strip"
	ActionStartTacticalTimer   EventType = "start_tactical_timer"
	ActionMoveTacticalStrip    EventType = "move_tactical_strip"
)

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
