package main

import (
	"log"
)

func (s *Server) frontEndEventHandler(client *FrontendClient, event Event) (interface{}, error) {
	// TODO: SwitchCase for different types of messages
	// TODO: Decide whether the responses are handled here or whether they are handled in the frontEndEvents function <- so far it is inside the handler.
	// TODO: In order for there to be non broadcast messages it is just done here?
	switch event.Type {
	/*
		case CloseConnection:
			log.Println("Close Connection Event")
			err := s.frontendeventhandlerCloseConnection(event)
			if err != nil {
				return nil, err
			}
			response := []byte("Connection Closed")
			return response, nil
	*/
	case GoAround:
		// This event is sent to all FrontEndClients - No need for any backend parsing
		err := s.frontendeventhandlerGoARound(event)
		return nil, err
	// TODO:
	case StripUpdate:
		print("Not Implemented")

	case StripTransferRequestInit:
		print("Not Implemented")

	case StripTransferRequestReject:
		print("Not Implemented")

	case StripMoveRequest:
		print("Not Implemented")

	default:
		log.Println("Unknown Event Type")
		response := []byte("Not sure what to do here - Unknown EventType Handler")
		return response, nil
	}
	// TODO: Other Events

	// TODO: Better managing Return responses.

	// TODO: Not sure what to do here.
	return nil, nil
}

