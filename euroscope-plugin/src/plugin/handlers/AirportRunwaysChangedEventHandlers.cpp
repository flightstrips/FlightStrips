#include "AirportRunwaysChangedEventHandlers.h"

void FlightStrips::handlers::AirportRunwaysChangedEventHandlers::Clear() {
    m_handlers.clear();
}

void FlightStrips::handlers::AirportRunwaysChangedEventHandlers::OnAirportRunwayActivityChanged() {
    for (const auto & m_handler : this->m_handlers) {
        m_handler->OnAirportRunwayActivityChanged();
    }
}

void FlightStrips::handlers::AirportRunwaysChangedEventHandlers::RegisterHandler(
        const std::shared_ptr<AirportRunwaysChangedEvent> &handler) {
    m_handlers.push_back(handler);
}
