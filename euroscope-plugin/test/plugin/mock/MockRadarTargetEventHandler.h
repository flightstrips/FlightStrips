#pragma once
#include <gmock/gmock.h>
#include "handlers/RadarTargetEventHandler.h"

class MockRadarTargetEventHandler : public FlightStrips::handlers::RadarTargetEventHandler {
public:
    MOCK_METHOD2(RadarTargetPositionEvent, void(EuroScopePlugIn::CRadarTarget, bool));
    MOCK_METHOD1(RadarTargetOutOfRangeEvent, void(EuroScopePlugIn::CRadarTarget));
};
