//
// Created by fsr19 on 17/02/2024.
//

#include "TimedEventHandlers.h"

void FlightStrips::handlers::TimedEventHandlers::RegisterHandler(const std::shared_ptr<TimedEventHandler> &handler) {
    this->m_handlers.push_back(handler);
}

void FlightStrips::handlers::TimedEventHandlers::Clear() {
    this->m_handlers.clear();
}

void FlightStrips::handlers::TimedEventHandlers::OnTimer(int time) {
    for (const auto & m_handler : this->m_handlers) {
        m_handler->OnTimer(time);
    }
}
