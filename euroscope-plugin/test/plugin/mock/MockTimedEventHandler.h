#pragma once
#include <gmock/gmock.h>
#include "handlers/TimedEventHandler.h"

class MockTimedEventHandler : public FlightStrips::handlers::TimedEventHandler {
public:
    MOCK_METHOD1(OnTimer, void(int));
};
