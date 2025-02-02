
#pragma once

#include <unordered_map>
#include "handlers/RadarTargetEventHandler.h"
#include "FlightPlan.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "stands/StandService.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips::flightplan {
class FlightPlanService final : public handlers::FlightPlanEventHandler, public handlers::RadarTargetEventHandler  {
    public:

    explicit FlightPlanService(const std::shared_ptr<websocket::WebSocketService> &websocketService, const std::shared_ptr<FlightStripsPlugin> &flightStripsPlugin, const std::shared_ptr<stands::StandService>& standService);

    void RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) override;

    void FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

    void ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) override;

    void FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

private:
    std::shared_ptr<websocket::WebSocketService> m_websocketService;
    std::shared_ptr<FlightStripsPlugin> m_flightStripsPlugin;
    std::shared_ptr<stands::StandService> m_standService;
    std::unordered_map<std::string, FlightPlan> m_flightPlans = {};

    static std::string GetEstimatedLandingTime(const EuroScopePlugIn::CFlightPlan& flightPlan);


};
}
