package frontend

import "FlightStrips/internal/aman"

const (
	AMANRebuildArrivalSequenceType EventType = "aman.rebuild_arrival_sequence"
	AMANArrivalSequenceRebuiltType EventType = "aman.arrival_sequence_rebuilt"
)

type AMANRebuildArrivalSequenceMessage struct {
	Type    EventType       `json:"type"`
	Version int             `json:"version"`
	Data    AMANCommandMeta `json:"data"`
}

type AMANArrivalSequenceRebuiltEvent struct {
	Version int                              `json:"version"`
	Data    AMANArrivalSequenceRebuiltResult `json:"data"`
}

type AMANArrivalSequenceRebuiltResult struct {
	CommandID         string `json:"command_id"`
	Revision          uint64 `json:"revision"`
	Changed           bool   `json:"changed"`
	Duplicate         bool   `json:"duplicate"`
	PublicationStatus string `json:"publication_status"`
	aman.ArrivalSequenceRebuildOutcome
}

func (AMANArrivalSequenceRebuiltEvent) GetType() EventType         { return AMANArrivalSequenceRebuiltType }
func (e AMANArrivalSequenceRebuiltEvent) Marshal() ([]byte, error) { return marshall(e) }
