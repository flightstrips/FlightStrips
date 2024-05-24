#pragma once

#include "ControllerEventHandler.h"

namespace FlightStrips::handlers {
    class ControllerEventHandlers {
    public:
        void Clear();
        void ControllerPositionUpdateEvent(EuroScopePlugIn::CController controller) const;
        void ControllerDisconnectEvent(EuroScopePlugIn::CController controller) const;

        void RegisterHandler(const std::shared_ptr<ControllerEventHandler>& handler);

    private:
        std::list<std::shared_ptr<ControllerEventHandler>> m_handlers;
    };
}
