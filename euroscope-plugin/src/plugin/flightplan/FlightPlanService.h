
#pragma once

#include <unordered_map>
#include "handlers/RadarTargetEventHandler.h"
#include "FlightPlan.h"
#include "handlers/FlightPlanEventHandlers.h"

namespace FlightStrips::flightplan {
class FlightPlanService : public handlers::RadarTargetEventHandler {
    public:

    explicit FlightPlanService(const std::shared_ptr<handlers::FlightPlanEventHandlers> &handlers);

    void RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) override;

    private:
        std::unordered_map<std::string, FlightPlan> m_flightPlans;

        std::shared_ptr<handlers::FlightPlanEventHandlers> handlers;
    };
}