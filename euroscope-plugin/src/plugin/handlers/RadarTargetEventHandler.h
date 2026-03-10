#pragma once

namespace FlightStrips::handlers {
    class RadarTargetEventHandler {
    public:
        virtual ~RadarTargetEventHandler() = default;

        virtual void RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget, bool isRangeOnly) = 0;
        virtual void RadarTargetOutOfRangeEvent(EuroScopePlugIn::CRadarTarget radarTarget) = 0;
    };
}
