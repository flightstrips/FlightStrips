package main

type EuroscopeEventHandler func(client *EuroscopeClient, message []byte) error

type EuroscopeEventHandlers struct {
	Handlers map[EventType]EuroscopeEventHandler
}

func GetEuroscopeEventHandlers() EuroscopeEventHandlers {
	handlers := make(map[EventType]EuroscopeEventHandler)

	handlers[EuroscopeControllerOnline] = euroscopeeventhandlerControllerOnline
	handlers[EuroscopeControllerOffline] = euroscopeeventhandlerControllerOffline
	handlers[EuroscopeSync] = euroscopeeventhandlerSync
	handlers[EuroscopeAssignedSquawk] = euroscopeeventhandlerAssignedSquawk
	handlers[EuroscopeSquawk] = euroscopeeventhandlerSquawk
	handlers[EuroscopeRequestedAltitude] = euroscopeeventhandlerRequestedAltitude
	handlers[EuroscopeClearedAltitude] = euroscopeeventhandlerClearedAltitude
	handlers[EuroscopeCommunicationType] = euroscopeeventhandlerCommunicationType
	handlers[EuroscopeGroundState] = euroscopeeventhandlerGroundState
	handlers[EuroscopeClearedFlag] = euroscopeeventhandlerClearedFlag
	handlers[EuroscopePositionUpdate] = euroscopeeventhandlerPositionUpdate
	handlers[EuroscopeSetHeading] = euroscopeeventhandlerSetHeading
	handlers[EuroscopeAircraftDisconnected] = euroscopeeventhandlerAircraftDisconnected
	handlers[EuroscopeStand] = euroscopeeventhandlerStand
	handlers[EuroscopeStripUpdate] = euroscopeeventhandlerStripUpdate

	return EuroscopeEventHandlers{
		handlers,
	}
}
