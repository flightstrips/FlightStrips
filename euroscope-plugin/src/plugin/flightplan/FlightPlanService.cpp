#include "FlightPlanService.h"

namespace FlightStrips::flightplan {

    void FlightPlanService::RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) {
        FlightPlan plan;
        plan.squawk = radarTarget.GetPosition().GetSquawk();

        auto entry = this->m_flightPlans.insert({radarTarget.GetCallsign(), plan});

        if (!entry.second) {
            if (entry.first->second.squawk == plan.squawk) {
                return;
            }
            entry.first->second.squawk = plan.squawk;
        }

        this->handlers->SquawkUpdateEvent(radarTarget.GetCallsign(), plan.squawk);
    }

    FlightPlanService::FlightPlanService(const std::shared_ptr<handlers::FlightPlanEventHandlers> &handlers) : handlers(
            handlers) {}
}