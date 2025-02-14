#pragma once

namespace FlightStrips::handlers {
    class AuthenticationEventHandler {
    public:
        virtual ~AuthenticationEventHandler() = default;
        virtual void OnTokenUpdate(const std::string& token) = 0;
    };
}
