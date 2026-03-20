#pragma once
#include <gmock/gmock.h>
#include "handlers/ConnectionEventHandler.h"

class MockConnectionEventHandler : public FlightStrips::handlers::ConnectionEventHandler {
public:
    MOCK_METHOD0(Online, void());
};
