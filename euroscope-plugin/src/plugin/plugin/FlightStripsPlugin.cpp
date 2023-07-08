
#include <format>
#include "FlightStripsPlugin.h"
#include "euroscope/EuroScopePlugIn.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "runway/ActiveRunway.h"

using namespace EuroScopePlugIn;

namespace FlightStrips {
    FlightStripsPlugin::FlightStripsPlugin(
            const std::shared_ptr<handlers::FlightPlanEventHandlers> &mFlightPlanEventHandlerCollection,
            const std::shared_ptr<handlers::RadarTargetEventHandlers> &mRadarTargetEventHandlers,
            const std::shared_ptr<network::NetworkService> mNetworkService)
            : CPlugIn(COMPATIBILITY_CODE, PLUGIN_NAME, "0.0.1", PLUGIN_AUTHOR, PLUGIN_COPYRIGHT),
              m_flightPlanEventHandlerCollection(mFlightPlanEventHandlerCollection),
              m_radarTargetEventHandlers(mRadarTargetEventHandlers), m_networkService(mNetworkService) {
    }

    void FlightStripsPlugin::Information(const std::string& message) {
        DisplayUserMessage("message", PLUGIN_NAME, message.c_str(), true, false, false, false, false);
    }

    void FlightStripsPlugin::OnFlightPlanDisconnect(EuroScopePlugIn::CFlightPlan FlightPlan) {
        if (!IsRelevant(FlightPlan)) {
            return;
        }

        this->m_flightPlanEventHandlerCollection->FlightPlanDisconnectEvent(FlightPlan);
    }

    void FlightStripsPlugin::OnFlightPlanControllerAssignedDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan,
                                                                      int DataType) {
        if (!IsRelevant(FlightPlan)) {
            return;
        }

        this->m_flightPlanEventHandlerCollection->ControllerFlightPlanDataEvent(FlightPlan, DataType);
    }

    void FlightStripsPlugin::OnFlightPlanFlightPlanDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan) {
        if (!IsRelevant(FlightPlan)) {
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

    void FlightStripsPlugin::OnRadarTargetPositionUpdate(EuroScopePlugIn::CRadarTarget RadarTarget) {
        if (!RadarTarget.IsValid() || !IsRelevant(RadarTarget.GetCorrelatedFlightPlan())) {
            return;
        }

        this->m_radarTargetEventHandlers->RadarTargetPositionEvent(RadarTarget);
    }

    FlightStripsPlugin::~FlightStripsPlugin() = default;

    bool FlightStripsPlugin::IsRelevant(EuroScopePlugIn::CFlightPlan flightPlan) {
        return flightPlan.IsValid() &&
               (strcmp(flightPlan.GetFlightPlanData().GetDestination(), AIRPORT) == 0
                  || strcmp(flightPlan.GetFlightPlanData().GetOrigin(), AIRPORT) == 0);
    }

    void FlightStripsPlugin::OnAirportRunwayActivityChanged() {
        std::vector<runway::ActiveRunway> active;

        auto it = CPlugIn::SectorFileElementSelectFirst(SECTOR_ELEMENT_RUNWAY);
        while (it.IsValid()) {
            if (strncmp(it.GetAirportName(), "EKCH", 4) == 0) {
                for (int i = 0; i < 2; i++) {
                    for (int j = 0; j < 2; j++) {
                        if (it.IsElementActive((bool)j, i)) {
                            runway::ActiveRunway runway = { it.GetRunwayName(i), (bool)j };
                            active.push_back(runway);
                        }
                    }
                }
            }

            it = CPlugIn::SectorFileElementSelectNext(it, SECTOR_ELEMENT_RUNWAY);
        }

        this->m_networkService->SendActiveRunways(active);
    }

    void FlightStripsPlugin::SetClearenceFlag(std::string callsign, bool cleared) {
        if (cleared) {
            this->UpdateViaScratchPad(callsign.c_str(), CLEARED);
        } else {
            this->UpdateViaScratchPad(callsign.c_str(), NOT_CLEARED);
        }
    }

    void FlightStripsPlugin::UpdateViaScratchPad(const char* callsign, const char *message) const {
        auto fp = this->FlightPlanSelect(callsign);

        if (!fp.IsValid()) return;

        auto scratch = fp.GetControllerAssignedData().GetScratchPadString();
        fp.GetControllerAssignedData().SetScratchPadString(message);
        fp.GetControllerAssignedData().SetScratchPadString(scratch);
    }
}

