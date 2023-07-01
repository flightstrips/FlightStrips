
#pragma once

#include <memory>
#include "FlightPlanEventHandler.h"

namespace FlightStrips::handlers {
    class FlightPlanEventHandlers {
    public:
        void FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) const;
        void ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) const;
        void FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) const;
        void SquawkUpdateEvent(std::string callsign, int squawk) const;

        void RegisterHandler(const std::shared_ptr<FlightPlanEventHandler>& handler);

    private:
        std::list<std::shared_ptr<FlightPlanEventHandler>> m_handlers;
    };

}
