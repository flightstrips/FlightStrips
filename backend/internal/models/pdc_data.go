package models

import "time"

const (
	PdcStateNone    = "NONE"
	PdcChannelCPDLC = "CPDLC"
	PdcChannelWeb   = "WEB"
)

type PdcWebData struct {
	Atis                *string    `json:"atis,omitempty"`
	Stand               *string    `json:"stand,omitempty"`
	ClearanceText       *string    `json:"clearanceText,omitempty"`
	PilotAcknowledgedAt *time.Time `json:"pilotAcknowledgedAt,omitempty"`
}

type PdcData struct {
	State           string      `json:"state,omitempty"`
	RequestChannel  *string     `json:"requestChannel,omitempty"`
	RequestRemarks  *string     `json:"requestRemarks,omitempty"`
	RequestedAt     *time.Time  `json:"requestedAt,omitempty"`
	MessageSequence *int32      `json:"messageSequence,omitempty"`
	MessageSent     *time.Time  `json:"messageSent,omitempty"`
	IssuedByCid     *string     `json:"issuedByCid,omitempty"`
	Web             *PdcWebData `json:"web,omitempty"`
}

func (d *PdcData) Normalize() *PdcData {
	if d == nil {
		return &PdcData{State: PdcStateNone}
	}
	if d.State == "" {
		d.State = PdcStateNone
	}
	return d
}

func (d *PdcData) Clone() *PdcData {
	if d == nil {
		return (&PdcData{}).Normalize()
	}

	clone := *d
	if d.Web != nil {
		webClone := *d.Web
		clone.Web = &webClone
	}

	return clone.Normalize()
}
