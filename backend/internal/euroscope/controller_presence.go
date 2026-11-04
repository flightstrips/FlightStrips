package euroscope

import (
	"context"
	"sort"
	"strings"
	"time"

	"FlightStrips/internal/models"
	"FlightStrips/internal/shared"
	es "FlightStrips/pkg/events/euroscope"
	"google.golang.org/protobuf/proto"
)

type membershipControllerWriter interface {
	UpsertMembershipController(context.Context, *models.Controller, bool) error
}
type controllerPresence struct {
	cid, position string
	observer      bool
}

// Caller holds controllerRowLock. Inspect only immutable scope plus locked
// identity snapshots; never acquire a transport mutex in this lock order.
func (hub *Hub) controllerPresence(airport, environment, callsign string, incoming *controllerPresence) (controllerPresence, bool) {
	var candidates []controllerPresence
	for _, member := range hub.clientsSnapshot() {
		if member.isClosed() || !member.IsAuthenticated() || !strings.EqualFold(member.airport, airport) || !strings.EqualFold(member.sessionName, environment) || member.GetCallsign() != callsign {
			continue
		}
		identity := member.identitySnapshot()
		candidates = append(candidates, controllerPresence{cid: member.GetCid(), position: identity.position, observer: identity.observer})
	}
	if incoming != nil {
		candidates = append(candidates, *incoming)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].observer != candidates[j].observer {
			return !candidates[i].observer
		}
		return candidates[i].cid+"\x00"+candidates[i].position < candidates[j].cid+"\x00"+candidates[j].position
	})
	if len(candidates) == 0 {
		return controllerPresence{}, false
	}
	return candidates[0], true
}
func presenceRow(session int32, callsign string, p controllerPresence) *models.Controller {
	row := &models.Controller{Session: session, Callsign: callsign, Position: p.position, Observer: p.observer, Cid: &p.cid}
	if !p.observer {
		now := time.Now().UTC()
		row.LastSeenEuroscope = &now
	}
	return row
}
func (hub *Hub) admitMembershipController(ctx context.Context, login *es.LoginEvent, user shared.AuthenticatedUser) (es.LoginEvent, int32, error) {
	writer, ok := hub.server.GetControllerRepository().(membershipControllerWriter)
	if !ok {
		payload, err := proto.Marshal(login)
		if err != nil {
			return es.LoginEvent{}, 0, err
		}
		return hub.handleLogin(payload, user)
	}
	event := *login
	event.Connection = strings.ToUpper(strings.TrimSpace(event.Connection))
	session, err := hub.server.GetOrCreateSession(event.Airport, event.Connection)
	if err != nil {
		return event, 0, err
	}
	incoming := controllerPresence{cid: user.GetCid(), position: event.Position, observer: event.Observer}
	projected, _ := hub.controllerPresence(event.Airport, event.Connection, event.Callsign, &incoming)
	err = writer.UpsertMembershipController(ctx, presenceRow(session.Id, event.Callsign, projected), !event.Observer)
	return event, session.Id, err
}

// Caller holds the member application lock. Serialize position projection with
// admission and detach without acquiring another transport lock.
func (hub *Hub) updateMembershipPosition(ctx context.Context, c *Client, event *es.ControlEvent) error {
	rowLock := hub.controllerRowLock(c.airport, c.sessionName, c.GetCallsign())
	rowLock.Lock()
	defer rowLock.Unlock()
	before := c.identitySnapshot()
	c.updateIdentity(event.PrimaryFrequency, event.Callsign, event.Observer, event.LocalIp)
	var err error
	if writer, ok := hub.server.GetControllerRepository().(membershipControllerWriter); ok {
		projection, exists := hub.controllerPresence(c.airport, c.sessionName, c.GetCallsign(), nil)
		if exists {
			err = writer.UpsertMembershipController(ctx, presenceRow(c.session, c.GetCallsign(), projection), false)
		}
	} else {
		_, err = hub.server.GetControllerRepository().SetPosition(ctx, c.session, c.GetCallsign(), event.PrimaryFrequency)
	}
	if err != nil {
		c.updateIdentity(before.position, before.callsign, before.observer, before.localIP)
	}
	return err
}
