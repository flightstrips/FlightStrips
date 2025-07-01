package main

import (
	"fmt"
)

func (s *Server) frontEndEventHandler(client *FrontendClient, event Event, message []byte) error {
	switch event.Type {
	case GoAround:
		err := s.frontendeventhandlerGoARound(event)
		return err
	case FrontendMove:
		err := s.frontendEventHandlerMove(client, message)
		return err
	case FrontendGenerateSquawk:
		err := s.frontendEventGenerateSquawk(client, message)
		return err
	case FrontendUpdateStripData:
		err := s.frontendEventHandlerStripUpdate(client, message)
		return err
	case CoordinationTransferRequestType:
		err := s.frontendEventHandlerCoordinationTransferRequest(client, message)
		return err
	case CoordinationAssumeRequestType:
		err := s.frontendEventHandlerCoordinationAssumeRequest(client, message)
		return err
	case CoordinationRejectRequestType:
		err := s.frontendEventHandlerCoordinationRejectRequest(client, message)
		return err
	case CoordinationFreeRequestType:
		err := s.frontendEventHandlerCoordinationFreeRequest(client, message)
		return err
	default:
		return fmt.Errorf("unknown event type: %s", event.Type)
	}
}
