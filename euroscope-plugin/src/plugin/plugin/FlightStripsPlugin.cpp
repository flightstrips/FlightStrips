
#include <format>
#include "FlightStripsPlugin.h"
#include "euroscope/EuroScopePlugIn.h"
#include "handlers/FlightPlanEventHandlers.h"

using namespace EuroScopePlugIn;

namespace FlightStrips {
    FlightStripsPlugin::FlightStripsPlugin(
            const std::shared_ptr<handlers::FlightPlanEventHandlers> &mFlightPlanEventHandlerCollection)
            : CPlugIn(COMPATIBILITY_CODE, PLUGIN_NAME, "0.0.1", PLUGIN_AUTHOR, PLUGIN_COPYRIGHT),
              m_flightPlanEventHandlerCollection(mFlightPlanEventHandlerCollection) {
    }

    void FlightStripsPlugin::Information(const std::string& message) {
        DisplayUserMessage("message", PLUGIN_NAME, message.c_str(), true, false, false, false, false);
    }

    void FlightStripsPlugin::OnFlightPlanDisconnect(EuroScopePlugIn::CFlightPlan FlightPlan) {
        if (!FlightPlan.IsValid()) {
            return;
        }

        this->m_flightPlanEventHandlerCollection->FlightPlanDisconnectEvent(FlightPlan);
    }

    void FlightStripsPlugin::OnFlightPlanControllerAssignedDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan,
                                                                      int DataType) {
        if (!FlightPlan.IsValid()) {
            return;
        }

        this->m_flightPlanEventHandlerCollection->ControllerFlightPlanDataEvent(FlightPlan, DataType);
    }

    void FlightStripsPlugin::OnFlightPlanFlightPlanDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan) {
        if (!FlightPlan.IsValid()) {
            return;
        }

        this->m_flightPlanEventHandlerCollection->FlightPlanEvent(FlightPlan);
    }

    void FlightStripsPlugin::OnTimer(int Counter) {
        /*
        for (const auto& message : this->server->ReadMessages()) {
            Information(message);
        }
        */
    }

    void FlightStripsPlugin::OnFlightPlanFlightStripPushed(EuroScopePlugIn::CFlightPlan FlightPlan,
                                                              const char *sSenderController,
                                                              const char *sTargetController) {
        Information(FlightPlan.GetCallsign());

    }

    FlightStripsPlugin::~FlightStripsPlugin() = default;

}

