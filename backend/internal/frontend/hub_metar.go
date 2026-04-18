package frontend

import "FlightStrips/pkg/events/frontend"

func (hub *Hub) SendAtisUpdate(session int32, metar string, arrAtisCode string, depAtisCode string) {
	hub.metarMu.Lock()
	hub.metarCache[session] = metar
	hub.arrAtisCodeCache[session] = arrAtisCode
	hub.depAtisCodeCache[session] = depAtisCode
	hub.metarMu.Unlock()

	hub.Broadcast(session, frontend.AtisUpdateEvent{Metar: metar, ArrAtisCode: arrAtisCode, DepAtisCode: depAtisCode})
}
