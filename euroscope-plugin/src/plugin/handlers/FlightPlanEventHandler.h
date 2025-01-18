#pragma once

namespace FlightStrips::handlers {
    class FlightPlanEventHandler {
    public:
        virtual ~FlightPlanEventHandler() = default;
        virtual void FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) = 0;
        virtual void ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) = 0;
        virtual void FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) = 0;
        virtual void SquawkUpdateEvent(std::string callsign, std::string squawk) = 0;
    };
}