#pragma once
#include <gmock/gmock.h>
#include "handlers/AirportRunwaysChangedEvent.h"

class MockAirportRunwaysChangedEventHandler : public FlightStrips::handlers::AirportRunwaysChangedEvent {
public:
    MOCK_METHOD0(OnAirportRunwayActivityChanged, void());
};
