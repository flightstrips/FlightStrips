#pragma once
#include <gmock/gmock.h>
#include "handlers/ControllerEventHandler.h"

class MockControllerEventHandler : public FlightStrips::handlers::ControllerEventHandler {
public:
    MOCK_METHOD1(ControllerPositionUpdateEvent, void(EuroScopePlugIn::CController));
    MOCK_METHOD1(ControllerDisconnectEvent, void(EuroScopePlugIn::CController));
};
