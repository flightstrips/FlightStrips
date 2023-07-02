#pragma once

namespace FlightStrips::handlers {
    class AirportRunwaysChangedEvent {
        virtual void OnAirportRunwayActivityChanged() = 0;
    };
}