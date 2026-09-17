package models

// StripPublicationSnapshot is a single database snapshot used only while
// assembling one frontend publication. It must not be reused for mutations.
type StripPublicationSnapshot struct {
	Strip               *Strip
	Session             *Session
	Controllers         []*Controller
	SectorOwners        []*SectorOwner
	Assignments         []*StandAssignment
	CoordinationPending bool
}
