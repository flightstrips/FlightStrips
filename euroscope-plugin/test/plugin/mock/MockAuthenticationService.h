#pragma once
#include <gmock/gmock.h>
#include "authentication/IAuthenticationService.h"
#include "authentication/AuthenticationService.h"

class MockAuthenticationService : public FlightStrips::authentication::IAuthenticationService {
public:
    MOCK_METHOD(FlightStrips::authentication::AuthenticationState, GetAuthenticationState, (), (const, override));
    MOCK_METHOD(std::string, GetAccessToken, (), (const, override));
};
