package cdm

import (
	"FlightStrips/internal/repository"
	"FlightStrips/internal/shared"
	euroscopeEvents "FlightStrips/pkg/events/euroscope"
	frontendEvents "FlightStrips/pkg/events/frontend"
	"context"
)

type testCdmPublisher struct{}

func (testCdmPublisher) SendCdmUpdate(int32, frontendEvents.CdmDataEvent)    {}
func (testCdmPublisher) SendCdmUpdates(int32, []frontendEvents.CdmDataEvent) {}
func (testCdmPublisher) SendCdmWait(int32, string)                           {}

type testCdmEuroscope struct{}

func (testCdmEuroscope) Broadcast(int32, euroscopeEvents.OutgoingMessage)            {}
func (testCdmEuroscope) BroadcastCdmUpdates(int32, []euroscopeEvents.CdmUpdateEvent) {}
func (testCdmEuroscope) GetMasterCallsign(int32) string                              { return "" }
func (testCdmEuroscope) SendEobt(int32, string, string, string)                      {}

type testCdmValidationReevaluator struct{}

func (testCdmValidationReevaluator) ReevaluateCtotValidation(context.Context, int32, string, bool, bool) error {
	return nil
}

func (testCdmValidationReevaluator) ReevaluateCtotValidationsForSession(context.Context, int32, bool) error {
	return nil
}

func newTestCdmService(client *Client, stripRepo CdmStripStore, sessionRepo repository.SessionRepository, controllerRepo repository.ControllerRepository) *Service {
	return newCdmService(
		client,
		stripRepo,
		sessionRepo,
		controllerRepo,
		testCdmPublisher{},
		testCdmEuroscope{},
		testCdmValidationReevaluator{},
	)
}

func newTestSequenceService(stripRepo CdmSequenceStripStore, sessionRepo repository.SessionRepository, configProvider ConfigProvider, frontend shared.CdmEventPublisher, euroscope CdmEuroscope) *SequenceService {
	if frontend == nil {
		frontend = testCdmPublisher{}
	}
	if euroscope == nil {
		euroscope = testCdmEuroscope{}
	}
	return newSequenceService(stripRepo, sessionRepo, configProvider, frontend, euroscope)
}

func setTestCdmFrontend(service *Service, publisher shared.CdmEventPublisher) {
	service.publisher = publisher
}

func setTestCdmEuroscope(service *Service, euroscope CdmEuroscope) {
	service.euroscopeHub = euroscope
}

func setTestCdmValidation(service *Service, reevaluator StripValidationReevaluator) {
	service.validationReevaluator = reevaluator
}
