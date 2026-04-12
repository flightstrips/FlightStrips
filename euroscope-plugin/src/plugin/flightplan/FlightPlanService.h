#pragma once

#include <string>
#include <unordered_map>
#include <unordered_set>
#include <vector>
#include "handlers/RadarTargetEventHandler.h"
#include "FlightPlan.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "stands/StandService.h"
#include "websocket/WebSocketService.h"
#include "websocket/Events.h"
#include "handlers/TimedEventHandler.h"
#include "plugin/FlightStripsPlugin.h"

namespace FlightStrips::flightplan {
class FlightPlanService final : public handlers::FlightPlanEventHandler, public handlers::RadarTargetEventHandler, public handlers::TimedEventHandler  {
    public:

    explicit FlightPlanService(const std::shared_ptr<websocket::WebSocketService> &websocketService,
                              const std::shared_ptr<FlightStripsPlugin> &flightStripsPlugin,
                              const std::shared_ptr<stands::StandService>& standService,
                              const std::shared_ptr<configuration::AppConfig>& appConfig);

    void RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget, bool isRangeOnly) override;
    void RadarTargetOutOfRangeEvent(EuroScopePlugIn::CRadarTarget radarTarget) override;

    void FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

    void ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) override;

    void FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

    void OnTimer(int counter) override;

    FlightPlan* GetFlightPlan(const std::string &callsign);

    void SetStand(const std::string& callsign, const std::string& stand);
    void ApplyCdmUpdate(const CdmUpdateEvent& event);
    void ApplyBackendSyncCdm(const std::string& callsign, const BackendSyncCdmData& cdmData);
    void ApplyPdcStateChange(const std::string& callsign, const std::string& state, const std::string& requestRemarks = {});

    static std::string GetEstimatedLandingTime(const EuroScopePlugIn::CFlightPlan& flightPlan);
private:

    std::shared_ptr<websocket::WebSocketService> m_websocketService;
    std::shared_ptr<FlightStripsPlugin> m_flightStripsPlugin;
    std::shared_ptr<stands::StandService> m_standService;
    std::shared_ptr<configuration::AppConfig> m_appConfig;
    std::unordered_map<std::string, FlightPlan> m_flightPlans = {};
    std::unordered_map<std::string, PositionEvent> m_pendingPositionUpdates = {};
    std::unordered_set<std::string> m_rangeTrackedCallsigns = {};
    int m_lastPositionFlushCounter = 0;

    void FlushPositionUpdates();
};
}
