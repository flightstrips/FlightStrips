#pragma once

#include <chrono>
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

    static std::string GetEstimatedLandingTime(const EuroScopePlugIn::CFlightPlan& flightPlan);
private:
    friend class FlightPlanServiceLocalCdmTestAccessor;

    struct LocalCdmSnapshot {
        std::string asrt{};
        std::string tsac{};
        std::string tobt{};
        std::string tsat{};
        std::string ttot{};
        std::string ctot{};
        std::string manual_ctot{};

        [[nodiscard]] bool HasSendableValues() const;
        auto operator==(const LocalCdmSnapshot& other) const -> bool = default;
    };

    struct LocalCdmObservationWindow {
        std::chrono::steady_clock::time_point expires_at{};
        LocalCdmSnapshot last_observed{};
        bool has_observation = false;
        int stable_polls = 0;
    };

    std::shared_ptr<websocket::WebSocketService> m_websocketService;
    std::shared_ptr<FlightStripsPlugin> m_flightStripsPlugin;
    std::shared_ptr<stands::StandService> m_standService;
    std::shared_ptr<configuration::AppConfig> m_appConfig;
    std::unordered_map<std::string, FlightPlan> m_flightPlans = {};
    std::unordered_map<std::string, PositionEvent> m_pendingPositionUpdates = {};
    std::unordered_set<std::string> m_rangeTrackedCallsigns = {};
    std::unordered_map<std::string, LocalCdmSnapshot> m_lastSentLocalCdm = {};
    std::unordered_map<std::string, LocalCdmObservationWindow> m_localCdmObservationWindows = {};
    int m_lastPositionFlushCounter = 0;

    void FlushPositionUpdates();
    void ObserveQueuedLocalCdmRequests();
    void PollLocalCdmObservationWindows();
    [[nodiscard]] bool HasActiveLocalCdmObservationWindow(const std::string& callsign) const;
    void RefreshLocalCdmObservationWindow(const std::string& callsign, const std::string& reason);
    LocalCdmSnapshot ObserveLocalCdmFlightPlan(EuroScopePlugIn::CFlightPlan flightPlan, const std::string& reason);
    void ForgetLocalCdmState(const std::string& callsign);
    static LocalCdmSnapshot ParseLocalCdmAnnotation(const std::string& annotation);
    static std::string TrimWhitespace(std::string value);
    static std::vector<std::string> SplitSlashFields(const std::string& value);

};
}
