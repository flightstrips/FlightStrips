#pragma once

#include "euroscope/EuroScopePlugIn.h"

namespace FlightStrips::handlers {
    class RadarTargetEventHandler {
    public:
        virtual ~RadarTargetEventHandler() = default;

        virtual void RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) = 0;
    };
}
