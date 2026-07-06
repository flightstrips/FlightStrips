package services

import "FlightStrips/internal/shared"

type ControllerServiceOption func(*ControllerService)

func WithFrontendNotifier(frontendNotifier FrontendNotifier) ControllerServiceOption {
	return func(cs *ControllerService) {
		cs.frontendNotifier = frontendNotifier
	}
}

func WithSessionRecalculator(sessionRecalculator SessionRecalculator) ControllerServiceOption {
	return func(cs *ControllerService) {
		cs.sessionRecalculator = sessionRecalculator
	}
}

func WithControllerStripService(stripService shared.StripService) ControllerServiceOption {
	return func(cs *ControllerService) {
		cs.stripService = stripService
	}
}
