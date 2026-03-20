#pragma once
#include <gmock/gmock.h>
#include "handlers/FlightPlanEventHandler.h"

class MockFlightPlanEventHandler : public FlightStrips::handlers::FlightPlanEventHandler {
public:
    MOCK_METHOD1(FlightPlanEvent, void(EuroScopePlugIn::CFlightPlan));
    MOCK_METHOD2(ControllerFlightPlanDataEvent, void(EuroScopePlugIn::CFlightPlan, int));
    MOCK_METHOD1(FlightPlanDisconnectEvent, void(EuroScopePlugIn::CFlightPlan));
};
