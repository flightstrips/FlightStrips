#pragma once

#include "euroscope/EuroScopePlugIn.h"

namespace FlightStrips::handlers {
    class ControllerEventHandler {
    public:
        virtual ~ControllerEventHandler() = default;

        virtual void ControllerPositionUpdateEvent(EuroScopePlugIn::CController controller) = 0;
        virtual void ControllerDisconnectEvent(EuroScopePlugIn::CController controller) = 0;
    };
}
