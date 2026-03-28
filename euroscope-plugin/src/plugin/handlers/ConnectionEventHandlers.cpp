#include "ConnectionEventHandlers.h"

#include "ExceptionHandling.h"

namespace FlightStrips::handlers {
    void ConnectionEventHandlers::Clear() {
        m_handlers.clear();
    }

    void ConnectionEventHandlers::OnOnline() const {
        for (const auto & m_handler : this->m_handlers) {
            exceptions::RunGuarded("ConnectionEventHandlers::OnOnline", [m_handler] {
                m_handler->Online();
            });
        }
    }

    void ConnectionEventHandlers::RegisterHandler(const std::shared_ptr<ConnectionEventHandler> &handler) {
        m_handlers.push_back(handler);
    }
} // handlers
// FlightStrip
