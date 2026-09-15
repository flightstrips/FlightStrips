package models

// PositionSnapshot is a database-consistent view used by one surveillance report.
// A nil assignment is a known absence, not an instruction to fetch it again.
type PositionSnapshot struct {
	Strip      *Strip
	Assignment *StandAssignment
}
