#pragma once

namespace FlightStrips::handlers {
    class AirportRunwaysChangedEvent {
    public:
        virtual void OnAirportRunwayActivityChanged() = 0;
    };
}