
#include <format>
#include "FlightStripsPlugin.h"
#include "euroscope/EuroScopePlugIn.h"
#include "graphics/InfoScreen.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "runway/ActiveRunway.h"

using namespace EuroScopePlugIn;

namespace FlightStrips {
    FlightStripsPlugin::FlightStripsPlugin(
            const std::shared_ptr<handlers::FlightPlanEventHandlers> &mFlightPlanEventHandlerCollection,
            const std::shared_ptr<handlers::RadarTargetEventHandlers> &mRadarTargetEventHandlers,
            const std::shared_ptr<handlers::ControllerEventHandlers> &mControllerEventHandlers,
            const std::shared_ptr<handlers::TimedEventHandlers> &mTimedEventHandlers,
            const std::shared_ptr<handlers::AirportRunwaysChangedEventHandlers> &mAirportRunwaysChangedEventHandlers,
            const std::shared_ptr<authentication::AuthenticationService> &mAuthenticationService,
            const std::shared_ptr<configuration::UserConfig> &mUserConfig)
            : CPlugIn(COMPATIBILITY_CODE, PLUGIN_NAME, PLUGIN_VERSION, PLUGIN_AUTHOR, PLUGIN_COPYRIGHT),
              m_flightPlanEventHandlerCollection(mFlightPlanEventHandlerCollection),
              m_radarTargetEventHandlers(mRadarTargetEventHandlers),
              m_controllerEventHandlerCollection(mControllerEventHandlers),
              m_timedEventHandlers(mTimedEventHandlers),
              m_airportRunwayChangedEventHandlers(mAirportRunwaysChangedEventHandlers),
              m_authenticationService(mAuthenticationService),
              m_userConfig(mUserConfig)
    {
    }

    void FlightStripsPlugin::Information(const std::string &message) {
        DisplayUserMessage("FlightStrips", PLUGIN_NAME, message.c_str(), true, false, false, false, false);
    }

    void FlightStripsPlugin::Error(const std::string &message) {
        DisplayUserMessage("FlightStrips", PLUGIN_NAME, message.c_str(), true, true, true, true, true);
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

    void FlightStripsPlugin::OnTimer(int time) {
        m_timedEventHandlers->OnTimer(time);
    }

    void FlightStripsPlugin::OnFlightPlanFlightStripPushed(EuroScopePlugIn::CFlightPlan, const char *, const char *) {
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
        m_airportRunwayChangedEventHandlers->OnAirportRunwayActivityChanged();
    }

    void FlightStripsPlugin::SetClearenceFlag(const std::string &callsign, const bool cleared) {
        if (cleared) {
            this->UpdateViaScratchPad(callsign.c_str(), CLEARED);
        } else {
            this->UpdateViaScratchPad(callsign.c_str(), NOT_CLEARED);
        }
    }

    void FlightStripsPlugin::UpdateViaScratchPad(const char *callsign, const char *message) const {
        auto fp = this->FlightPlanSelect(callsign);

        if (!fp.IsValid()) return;

        auto scratch = std::string(fp.GetControllerAssignedData().GetScratchPadString());
        fp.GetControllerAssignedData().SetScratchPadString(message);
        fp.GetControllerAssignedData().SetScratchPadString(scratch.c_str());
    }

    std::vector<runway::ActiveRunway> FlightStripsPlugin::GetActiveRunways(const char *airport) const {
         std::vector<runway::ActiveRunway> active;

        auto it = CPlugIn::SectorFileElementSelectFirst(SECTOR_ELEMENT_RUNWAY);
        while (it.IsValid()) {
            if (strncmp(it.GetAirportName(), airport, 4) == 0) {
                for (int i = 0; i < 2; i++) {
                    for (int j = 0; j < 2; j++) {
                        if (it.IsElementActive(static_cast<bool>(j), i)) {
                            runway::ActiveRunway runway = {it.GetRunwayName(i), static_cast<bool>(j)};
                            active.push_back(runway);
                        }
                    }
                }
            }

            it = CPlugIn::SectorFileElementSelectNext(it, SECTOR_ELEMENT_RUNWAY);
        }

        return active;
    }

    CRadarScreen * FlightStripsPlugin::OnRadarScreenCreated(const char *sDisplayName,
        bool NeedRadarContent, bool GeoReferenced, bool CanBeSaved, bool CanBeCreated) {
        return new graphics::InfoScreen(m_authenticationService, m_userConfig);
    }

    void FlightStripsPlugin::OnControllerPositionUpdate(EuroScopePlugIn::CController Controller) {
        this->m_controllerEventHandlerCollection->ControllerPositionUpdateEvent(Controller);
    }

    void FlightStripsPlugin::OnControllerDisconnect(EuroScopePlugIn::CController Controller) {
        this->m_controllerEventHandlerCollection->ControllerDisconnectEvent(Controller);
    }

    bool FlightStripsPlugin::ControllerIsMe(EuroScopePlugIn::CController controller, EuroScopePlugIn::CController me) {
        return controller.IsValid() && strcmp(controller.GetFullName(), me.GetFullName()) == 0 &&
               controller.GetCallsign() == me.GetCallsign();
    }
}

