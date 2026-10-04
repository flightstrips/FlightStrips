#pragma once
#include <string>

namespace FlightStrips::handlers {
    class ConnectionEventHandler {
    public:
        virtual ~ConnectionEventHandler() = default;
        virtual void Online() = 0;
        virtual void SessionChanged(const std::string& identity) {}
    };
}
