package models

type SectorOwner struct {
	ID         int32
	Session    int32
	Sector     []string
	Position   string
	Identifier string
}
