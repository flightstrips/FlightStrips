#pragma once

namespace FlightStrips::handlers {
    class ConnectionEventHandler {
    public:
        virtual ~ConnectionEventHandler() = default;
        virtual void Online() = 0;
    };
}
