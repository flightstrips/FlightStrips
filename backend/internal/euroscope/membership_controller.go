package euroscope

import "FlightStrips/internal/models"

// Select only a live authenticated member owned by the authenticated CID. An
// explicit instance/membership selection never falls back to CID-wide lookup.
func (hub *Hub) GetMembershipController(cid string, session int32, instance, membership string) *models.Controller {
	for _, client := range hub.clientsSnapshot() {
		if client.membership == nil || client.isClosed() || !client.IsAuthenticated() || client.GetCid() != cid || client.session != session || client.membership.InstanceId != instance || client.membership.MembershipId != membership {
			continue
		}
		identity := client.identitySnapshot()
		return &models.Controller{Session: session, Callsign: identity.callsign, Position: identity.position, Observer: identity.observer, Cid: &cid}
	}
	return nil
}
