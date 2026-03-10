package shared

// SectorChange describes a single sector whose owning position changed as a result
// of a controller coming online or going offline.
type SectorChange struct {
	SectorName   string // e.g. "DEL", "GND", "TWR"
	FromPosition string // human-readable position Name before the change; "" if no previous owner
	ToPosition   string // human-readable position Name after the change; "" if no new owner
}
