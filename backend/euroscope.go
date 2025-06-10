package main

import (
	"errors"
	"fmt"
)

func (s *Server) euroscopeEventsHandler(client *EuroscopeClient, event EuroscopeEvent, msg []byte) error {
	switch event.Type {
	case EuroscopeControllerOnline:
		return s.euroscopeeventhandlerControllerOnline(msg, client.session, client.airport)
	case EuroscopeControllerOffline:
		return s.euroscopeeventhandlerControllerOffline(msg, client.session, client.airport)
	case EuroscopeSync:
		return s.euroscopeeventhandlerSync(msg, client.session, client.airport)
	case EuroscopeAssignedSquawk:
		return s.euroscopeeventhandlerAssignedSquawk(msg, client.session)
	case EuroscopeSquawk:
		return s.euroscopeeventhandlerSquawk(msg, client.session)
	case EuroscopeRequestedAltitude:
		return s.euroscopeeventhandlerRequestedAltitude(msg, client.session)
	case EuroscopeClearedAltitude:
		return s.euroscopeeventhandlerClearedAltitude(msg, client.session)
	case EuroscopeCommunicationType:
		return s.euroscopeeventhandlerCommunicationType(msg, client.session)
	case EuroscopeGroundState:
		return s.euroscopeeventhandlerGroundState(msg, client.session)
	case EuroscopeClearedFlag:
		return s.euroscopeeventhandlerClearedFlag(msg, client.session)
	case EuroscopePositionUpdate:
		return s.euroscopeeventhandlerPositionUpdate(msg, client.session)
	case EuroscopeSetHeading:
		return s.euroscopeeventhandlerSetHeading(msg, client.session)
	case EuroscopeAircraftDisconnected:
		return s.euroscopeeventhandlerAircraftDisconnected(msg, client.session)
	case EuroscopeStand:
		return s.euroscopeeventhandlerStand(msg, client.session)
	case EuroscopeStripUpdate:
		return s.euroscopeeventhandlerStripUpdate(msg, client.session)
	case EuroscopeRunway:
		return errors.New("not implemented")
	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}
}
