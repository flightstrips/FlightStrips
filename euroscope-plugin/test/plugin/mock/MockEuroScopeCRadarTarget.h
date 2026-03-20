#pragma once
#include <gmock/gmock.h>
#include "euroscope/EuroScopeCRadarTargetInterface.h"

class MockEuroScopeCRadarTarget : public FlightStrips::euroscope::EuroScopeCRadarTargetInterface {
public:
    MOCK_CONST_METHOD0(GetCallsign, std::string());
    MOCK_CONST_METHOD0(GetLatitude, double());
    MOCK_CONST_METHOD0(GetLongitude, double());
    MOCK_CONST_METHOD0(GetGroundSpeed, int());
    MOCK_CONST_METHOD0(IsReported, bool());
};
