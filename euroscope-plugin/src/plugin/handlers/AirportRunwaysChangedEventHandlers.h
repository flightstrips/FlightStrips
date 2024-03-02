#pragma once

#include "AirportRunwaysChangedEvent.h"

namespace FlightStrips::handlers {
    class AirportRunwaysChangedEventHandlers {
    public:
        void Clear();
        void OnAirportRunwayActivityChanged();
        void RegisterHandler(const std::shared_ptr<AirportRunwaysChangedEvent>& handler);
    private:
        std::list<std::shared_ptr<AirportRunwaysChangedEvent>> m_handlers;
    };
}
