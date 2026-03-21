#pragma once
#include <gmock/gmock.h>
#include "handlers/MessageHandler.h"

class MockMessageHandler : public FlightStrips::handlers::MessageHandler {
public:
    MOCK_METHOD1(OnMessages, void(const std::vector<nlohmann::json>&));
};
