#pragma once
#include <gmock/gmock.h>
#include "plugin/IFlightStripsPlugin.h"
#include "plugin/FlightStripsPlugin.h"

class MockFlightStripsPlugin : public FlightStrips::IFlightStripsPlugin {
public:
    MOCK_METHOD(FlightStrips::ConnectionState&, GetConnectionState, (), (override));
    MOCK_METHOD(std::vector<std::string>, GetEligibleAirports, (), (const, override));
    MOCK_METHOD(std::optional<std::vector<FlightStrips::VisibleController>>, GetVisibleControllers, (), (const, override));
};
