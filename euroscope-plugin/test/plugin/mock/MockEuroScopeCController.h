#pragma once
#include <gmock/gmock.h>
#include "euroscope/EuroScopeCControllerInterface.h"

class MockEuroScopeCController : public FlightStrips::euroscope::EuroScopeCControllerInterface {
public:
    MOCK_CONST_METHOD0(GetCallsign, std::string());
    MOCK_CONST_METHOD0(GetPositionId, std::string());
    MOCK_CONST_METHOD0(GetFrequency, int());
    MOCK_CONST_METHOD0(IsValid, bool());
};
