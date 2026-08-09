#pragma once

#include <memory>

#include "RadarTargetEventHandler.h"

namespace FlightStrips {
    class FlightStripsPlugin;
}

namespace FlightStrips::handlers {
    class EkytApproachTrackingHandler final : public RadarTargetEventHandler {
    public:
        explicit EkytApproachTrackingHandler(std::shared_ptr<FlightStripsPlugin> plugin);

        void RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget, bool isRangeOnly) override;
        void RadarTargetOutOfRangeEvent(EuroScopePlugIn::CRadarTarget radarTarget) override;

    private:
        std::shared_ptr<FlightStripsPlugin> m_plugin;
    };
}
