#include "GroundTagReleaseHandler.h"

#include "Logger.hpp"
#include "plugin/FlightStripsPlugin.h"
#include "plugin/GroundTagReleasePolicy.h"

namespace FlightStrips::handlers {
    GroundTagReleaseHandler::GroundTagReleaseHandler(std::weak_ptr<FlightStripsPlugin> plugin)
        : m_plugin(std::move(plugin)) {
    }

    void GroundTagReleaseHandler::ControllerFlightPlanDataEvent(
        EuroScopePlugIn::CFlightPlan flightPlan, const int dataType) {
        if (dataType != EuroScopePlugIn::CTR_DATA_TYPE_SCRATCH_PAD_STRING
            || !flightPlan.IsValid() || flightPlan.GetSimulated()) return;

        const auto plugin = m_plugin.lock();
        if (!plugin || plugin->GetConnectionState().observer || !flightPlan.GetTrackingControllerIsMe()) return;

        const auto scratchPad = flightPlan.GetControllerAssignedData().GetScratchPadString();
        const auto request = ParseRequestOnFrequency(scratchPad == nullptr ? "" : scratchPad);
        if (!request || _stricmp(request->aircraftCallsign.c_str(), flightPlan.GetCallsign()) != 0) return;

        const auto owner = plugin->ControllerMyself();
        const auto requester = plugin->ControllerSelect(request->controllerCallsign.c_str());
        if (!owner.IsValid() || !owner.IsController() || !requester.IsValid() || !requester.IsController()) return;
        if (!ShouldReleaseGroundTag(owner.GetCallsign(), owner.GetFacility(), requester.GetCallsign(),
                                   requester.GetFacility(), plugin->GetConnectionState().observer,
                                   flightPlan.GetTrackingControllerIsMe())) return;

        if (!flightPlan.EndTracking()) {
            Logger::Warning("Failed to release {} following ROF from {}", request->aircraftCallsign, request->controllerCallsign);
            return;
        }
        Logger::Info("Released {} following ROF from {}", request->aircraftCallsign, request->controllerCallsign);
    }

    void GroundTagReleaseHandler::FlightPlanEvent(const EuroScopePlugIn::CFlightPlan) {
    }

    void GroundTagReleaseHandler::FlightPlanDisconnectEvent(const EuroScopePlugIn::CFlightPlan) {
    }
}
