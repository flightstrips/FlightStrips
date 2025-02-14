#include "AuthenticationEventHandlers.h"

namespace FlightStrips::handlers {
    void AuthenticationEventHandlers::Clear() {
        m_handlers.clear();
    }

    void AuthenticationEventHandlers::OnTokenUpdate(const std::string &token) const {
        for (const auto & m_handler : this->m_handlers) {
            m_handler->OnTokenUpdate(token);
        }
    }

    void AuthenticationEventHandlers::RegisterHandler(const std::shared_ptr<AuthenticationEventHandler> &handler) {
        m_handlers.push_back(handler);
    }
}
