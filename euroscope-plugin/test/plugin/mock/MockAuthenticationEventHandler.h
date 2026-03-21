#pragma once
#include <gmock/gmock.h>
#include "handlers/AuthenticationEventHandler.h"

class MockAuthenticationEventHandler : public FlightStrips::handlers::AuthenticationEventHandler {
public:
    MOCK_METHOD1(OnTokenUpdate, void(const std::string&));
};
