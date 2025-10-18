package shared

import (
	"FlightStrips/pkg/events"
)

type Hub[TMessage events.OutgoingMessage] interface {
	Broadcast(session int32, message TMessage)
	Send(session int32, cid string, message TMessage)
}
