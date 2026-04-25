package shared

import (
	"FlightStrips/internal/config"
	internalModels "FlightStrips/internal/models"
)

func IsOperationalController(controller *internalModels.Controller) bool {
	return controller != nil && !controller.Observer
}

func IsOperationalPositionController(controller *internalModels.Controller) bool {
	return IsOperationalController(controller) && config.CallsignHasOwnerPrefix(controller.Callsign)
}

func IsOperationalControllerForPosition(controller *internalModels.Controller, position *config.Position) bool {
	return controller != nil &&
		position != nil &&
		IsOperationalPositionController(controller)
}
