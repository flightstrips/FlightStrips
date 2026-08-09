#include "EkytApproachTrackingHandler.h"

#include "Logger.hpp"
#include "plugin/ApproachHandoffPolicy.h"
#include "plugin/FlightStripsPlugin.h"

namespace FlightStrips::handlers {
    EkytApproachTrackingHandler::EkytApproachTrackingHandler(std::shared_ptr<FlightStripsPlugin> plugin)
        : m_plugin(std::move(plugin)) {
    }

    void EkytApproachTrackingHandler::RadarTargetPositionEvent(
        const EuroScopePlugIn::CRadarTarget radarTarget,
        const bool) {
        if (m_plugin == nullptr || !radarTarget.IsValid()) return;

        const auto callsign = std::string(radarTarget.GetCallsign());
        auto flightPlan = m_plugin->FlightPlanSelect(callsign.c_str());
        if (!flightPlan.IsValid()) return;

        const auto radarPosition = radarTarget.GetPosition();
        const auto pressureAltitude = radarPosition.IsValid() ? radarPosition.GetPressureAltitude() : 0;
        const auto flightPlanState = flightPlan.GetState();
        const auto& connectionState = m_plugin->GetConnectionState();

        if (ShouldAutoAssumeAndDropEkytApproachHandoff(
            connectionState.callsign,
            connectionState.primary_frequency,
            connectionState.observer,
            flightPlanState == EuroScopePlugIn::FLIGHT_PLAN_STATE_TRANSFER_TO_ME_INITIATED,
            radarPosition.IsValid(),
            pressureAltitude)) {
            Logger::Info(
                "Automatically assuming and dropping {} handed to an EKYT approach frequency at {} ft",
                callsign,
                pressureAltitude
            );
            flightPlan.AcceptHandoff();
            if (!flightPlan.EndTracking()) {
                Logger::Warning("Failed to drop tracking for {} after automatic EKYT approach handoff acceptance", callsign);
            }
            return;
        }

        if (!ShouldAutoDropEkytApproachAircraft(
            connectionState.callsign,
            connectionState.primary_frequency,
            connectionState.observer,
            flightPlanState == EuroScopePlugIn::FLIGHT_PLAN_STATE_ASSUMED,
            flightPlan.GetTrackingControllerIsMe())) {
            return;
        }

        Logger::Info("Automatically dropping {} assumed on an EKYT approach frequency", callsign);
        if (!flightPlan.EndTracking()) {
            Logger::Warning("Failed to drop tracking for {} after it was assumed on an EKYT approach frequency", callsign);
        }
    }

    void EkytApproachTrackingHandler::RadarTargetOutOfRangeEvent(
        const EuroScopePlugIn::CRadarTarget) {
    }
}
