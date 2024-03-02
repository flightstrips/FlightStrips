#pragma once

namespace FlightStrips::handlers {
    class TimedEventHandler {
    public:
        virtual ~TimedEventHandler() = default;

        virtual void OnTimer(int time) = 0;
    };
}
