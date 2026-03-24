#pragma once
#include <gmock/gmock.h>
#include "plugin/IFlightStripsPlugin.h"
#include "plugin/FlightStripsPlugin.h"

class MockFlightStripsPlugin : public FlightStrips::IFlightStripsPlugin {
public:
    MOCK_METHOD(FlightStrips::ConnectionState&, GetConnectionState, (), (override));
};
