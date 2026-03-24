#pragma once
#include <string>

// AuthenticationState enum values are needed by callers (e.g. WebSocketService).
// Include AuthenticationService.h only for the enum — the interface itself stays
// dependency-light, but we need the full enum definition, not just a forward declaration.
namespace FlightStrips::authentication {
    enum AuthenticationState {
        NONE = 0,
        LOGIN = 1,
        REFRESH = 2,
        AUTHENTICATED = 3
    };

    class IAuthenticationService {
    public:
        virtual ~IAuthenticationService() = default;
        virtual AuthenticationState GetAuthenticationState() const = 0;
        virtual std::string GetAccessToken() const = 0;
    };
}
