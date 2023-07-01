#pragma once

#include "RadarTargetEventHandler.h"

namespace FlightStrips::handlers {
    class RadarTargetEventHandlers {

    public:

        void RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) const;

        void RegisterHandler(const std::shared_ptr<RadarTargetEventHandler>& handler);

    private:
        std::list<std::shared_ptr<RadarTargetEventHandler>> m_handlers;
    };
}
