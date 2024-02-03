#include "ControllerEventHandlers.h"

namespace FlightStrips::handlers {
    void ControllerEventHandlers::RegisterHandler(const std::shared_ptr<ControllerEventHandler> &handler) {
        this->m_handlers.push_back(handler);
    }

    void ControllerEventHandlers::ControllerPositionUpdateEvent(EuroScopePlugIn::CController controller) const {
        for (auto it = this->m_handlers.cbegin(); it != this->m_handlers.cend(); ++it) {
            (*it)->ControllerPositionUpdateEvent(controller);
        }
    }

    void ControllerEventHandlers::ControllerDisconnectEvent(EuroScopePlugIn::CController controller) const {
        for (auto it = this->m_handlers.cbegin(); it != this->m_handlers.cend(); ++it) {
            (*it)->ControllerDisconnectEvent(controller);
        }
    }
}
