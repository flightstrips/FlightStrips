#pragma once
#include <gmock/gmock.h>
#include "euroscope/EuroScopeCFlightPlanInterface.h"

class MockEuroScopeCFlightPlan : public FlightStrips::euroscope::EuroScopeCFlightPlanInterface {
public:
    MOCK_CONST_METHOD0(GetCallsign, std::string());
    MOCK_CONST_METHOD0(GetOrigin, std::string());
    MOCK_CONST_METHOD0(GetDestination, std::string());
    MOCK_CONST_METHOD0(GetRoute, std::string());
    MOCK_CONST_METHOD0(GetAircraftType, std::string());
    MOCK_CONST_METHOD0(GetAirline, std::string());
    MOCK_CONST_METHOD0(GetAssignedSquawk, std::string());
    MOCK_CONST_METHOD0(GetScratchPadString, std::string());
    MOCK_CONST_METHOD0(GetClearedAltitude, int());
    MOCK_CONST_METHOD0(IsTrackedByMe, bool());
    MOCK_METHOD1(SetScratchPadString, void(const std::string&));
    MOCK_METHOD1(SetSquawk, void(const std::string&));
};
