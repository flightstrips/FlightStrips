
#pragma once

#include <unordered_map>
#include "handlers/RadarTargetEventHandler.h"
#include "FlightPlan.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips::flightplan {
class FlightPlanService final : public handlers::FlightPlanEventHandler, public handlers::RadarTargetEventHandler  {
    public:

    explicit FlightPlanService(std::shared_ptr<websocket::WebSocketService> websocketService);

    void RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) override;

    void FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

    void ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) override;

    void FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

private:
    std::shared_ptr<websocket::WebSocketService> m_websocketService;
    std::unordered_map<std::string, FlightPlan> m_flightPlans = {};


};
}
