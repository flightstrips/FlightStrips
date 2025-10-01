package main

type FrontendEventHandler func(client *FrontendClient, message []byte) error

type FrontendEventHandlers struct {
	Handlers map[EventType]FrontendEventHandler
}

func GetFrontendEventHandlers() FrontendEventHandlers {
	handlers := make(map[EventType]FrontendEventHandler)

	handlers[GoAround] = frontendeventhandlerGoARound
	handlers[FrontendMove] = frontendEventHandlerMove
	handlers[FrontendGenerateSquawk] = frontendEventGenerateSquawk
	handlers[FrontendUpdateStripData] = frontendEventHandlerStripUpdate
	handlers[CoordinationTransferRequestType] = frontendEventHandlerCoordinationTransferRequest
	handlers[CoordinationAssumeRequestType] = frontendEventHandlerCoordinationAssumeRequest
	handlers[CoordinationRejectRequestType] = frontendEventHandlerCoordinationRejectRequest
	handlers[CoordinationFreeRequestType] = frontendEventHandlerCoordinationFreeRequest

	return FrontendEventHandlers{
		handlers,
	}
}
