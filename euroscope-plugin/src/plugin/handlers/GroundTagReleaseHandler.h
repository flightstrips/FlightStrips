#pragma once

#include <memory>

#include "FlightPlanEventHandler.h"

namespace FlightStrips {
    class FlightStripsPlugin;
}

namespace FlightStrips::handlers {
    class GroundTagReleaseHandler final : public FlightPlanEventHandler {
    public:
        explicit GroundTagReleaseHandler(std::weak_ptr<FlightStripsPlugin> plugin);

        void FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;
        void ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) override;
        void FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

    private:
        std::weak_ptr<FlightStripsPlugin> m_plugin;
    };
}
