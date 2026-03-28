#include "MessageHandlers.h"

#include "ExceptionHandling.h"

namespace FlightStrips::handlers {
    void MessageHandlers::Clear() {
        m_handlers.clear();
    }

    void MessageHandlers::OnMessages(const std::vector<nlohmann::json>& messages) const {
        for (const auto & m_handler : this->m_handlers) {
            exceptions::RunGuarded("MessageHandlers::OnMessages", [m_handler, &messages] {
                m_handler->OnMessages(messages);
            });
        }
    }

    void MessageHandlers::RegisterHandler(const std::shared_ptr<MessageHandler> &handler) {
        m_handlers.push_back(handler);
    }
}
