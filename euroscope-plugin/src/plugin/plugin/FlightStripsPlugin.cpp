
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
            const std::shared_ptr<handlers::ControllerEventHandlers> &mControllerEventHandlers,
            const std::shared_ptr<handlers::TimedEventHandlers> &mTimedEventHandlers,
            const std::shared_ptr<handlers::AirportRunwaysChangedEventHandlers> &mAirportRunwaysChangedEventHandlers)
            : CPlugIn(COMPATIBILITY_CODE, PLUGIN_NAME, "0.0.1", PLUGIN_AUTHOR, PLUGIN_COPYRIGHT),
              m_flightPlanEventHandlerCollection(mFlightPlanEventHandlerCollection),
              m_radarTargetEventHandlers(mRadarTargetEventHandlers),
              m_controllerEventHandlerCollection(mControllerEventHandlers),
              m_timedEventHandlers(mTimedEventHandlers),
              m_airportRunwayChangedEventHandlers(mAirportRunwaysChangedEventHandlers)
    {
    }

    void FlightStripsPlugin::Information(const std::string &message) {
        DisplayUserMessage("FlightStrips", PLUGIN_NAME, message.c_str(), true, false, false, false, false);
    }

    void FlightStripsPlugin::Error(const std::string &message) {
        DisplayUserMessage("FlightStrips", PLUGIN_NAME, message.c_str(), true, true, true, true, true);
    }

    void FlightStripsPlugin::OnFlightPlanDisconnect(EuroScopePlugIn::CFlightPlan FlightPlan) {
        try {
            if (!IsRelevant(FlightPlan)) {
                return;
            }

            this->m_flightPlanEventHandlerCollection->FlightPlanDisconnectEvent(FlightPlan);
        } catch (std::exception &e) {
            Error("Error during flight plant disconnect (" + std::string(FlightPlan.GetCallsign()) + "): " +
                        std::string(e.what()));
        }
    }

    void FlightStripsPlugin::OnFlightPlanControllerAssignedDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan,
                                                                      int DataType) {
        try {
            if (!IsRelevant(FlightPlan)) {
                return;
            }

            this->m_flightPlanEventHandlerCollection->ControllerFlightPlanDataEvent(FlightPlan, DataType);
        } catch (std::exception &e) {
            Error("Error during controller data update (" + std::string(FlightPlan.GetCallsign()) + "): " +
                        std::string(e.what()));
        }
    }

    void FlightStripsPlugin::OnFlightPlanFlightPlanDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan) {
        try {
            if (!IsRelevant(FlightPlan)) {
                return;
            }

            this->m_flightPlanEventHandlerCollection->FlightPlanEvent(FlightPlan);
        } catch (std::exception &e) {
            Error("Error during flight plan update (" + std::string(FlightPlan.GetCallsign()) + "): " +
                        std::string(e.what()));
        }
    }

    void FlightStripsPlugin::OnTimer(int time) {
        m_timedEventHandlers->OnTimer(time);
    }

    void FlightStripsPlugin::OnFlightPlanFlightStripPushed(EuroScopePlugIn::CFlightPlan, const char *, const char *) {
    }

    void FlightStripsPlugin::OnRadarTargetPositionUpdate(EuroScopePlugIn::CRadarTarget RadarTarget) {
        try {
            if (!RadarTarget.IsValid() || !IsRelevant(RadarTarget.GetCorrelatedFlightPlan())) {
                return;
            }

            this->m_radarTargetEventHandlers->RadarTargetPositionEvent(RadarTarget);
        } catch (std::exception &e) {
            Error("Error during radar position(" + std::string(RadarTarget.GetCallsign()) + "): " +
                        std::string(e.what()));
        }

    }

    FlightStripsPlugin::~FlightStripsPlugin() = default;

    bool FlightStripsPlugin::IsRelevant(EuroScopePlugIn::CFlightPlan flightPlan) {
        return flightPlan.IsValid() &&
               (strcmp(flightPlan.GetFlightPlanData().GetDestination(), AIRPORT) == 0
                || strcmp(flightPlan.GetFlightPlanData().GetOrigin(), AIRPORT) == 0);
    }

    void FlightStripsPlugin::OnAirportRunwayActivityChanged() {
        try {
            m_airportRunwayChangedEventHandlers->OnAirportRunwayActivityChanged();
            /*
            std::vector<runway::ActiveRunway> active;


            auto it = CPlugIn::SectorFileElementSelectFirst(SECTOR_ELEMENT_RUNWAY);
            while (it.IsValid()) {
                if (strncmp(it.GetAirportName(), "EKCH", 4) == 0) {
                    for (int i = 0; i < 2; i++) {
                        for (int j = 0; j < 2; j++) {
                            if (it.IsElementActive((bool) j, i)) {
                                runway::ActiveRunway runway = {it.GetRunwayName(i), (bool) j};
                                active.push_back(runway);
                            }
                        }
                    }
                }

                it = CPlugIn::SectorFileElementSelectNext(it, SECTOR_ELEMENT_RUNWAY);
            }
             */
        } catch (std::exception &e) {
            Error("Error during runway change: " + std::string(e.what()));
        }
    }

    void FlightStripsPlugin::SetClearenceFlag(std::string callsign, bool cleared) {
        try {
            if (cleared) {
                this->UpdateViaScratchPad(callsign.c_str(), CLEARED);
            } else {
                this->UpdateViaScratchPad(callsign.c_str(), NOT_CLEARED);
            }
        } catch (std::exception &e) {
            Error("Error during set clearance(" + callsign + "): " + std::string(e.what()));
        }
    }

    void FlightStripsPlugin::UpdateViaScratchPad(const char *callsign, const char *message) const {
        auto fp = this->FlightPlanSelect(callsign);

        if (!fp.IsValid()) return;

        auto scratch = std::string(fp.GetControllerAssignedData().GetScratchPadString());
        fp.GetControllerAssignedData().SetScratchPadString(message);
        fp.GetControllerAssignedData().SetScratchPadString(scratch.c_str());
    }

    void FlightStripsPlugin::OnControllerPositionUpdate(EuroScopePlugIn::CController Controller) {
        try {
            this->m_controllerEventHandlerCollection->ControllerPositionUpdateEvent(Controller);
        } catch (std::exception &e) {
            Error("Error during controller position update (" + std::string(Controller.GetCallsign()) + "): " +
                        std::string(e.what()));
        }
    }

    void FlightStripsPlugin::OnControllerDisconnect(EuroScopePlugIn::CController Controller) {
        try {
            this->m_controllerEventHandlerCollection->ControllerDisconnectEvent(Controller);
        } catch (std::exception &e) {
            Error("Error during controller disconnect (" + std::string(Controller.GetCallsign()) + "): " +
                        std::string(e.what()));
        }
    }

    bool FlightStripsPlugin::ControllerIsMe(EuroScopePlugIn::CController controller, EuroScopePlugIn::CController me) {
        return controller.IsValid() && strcmp(controller.GetFullName(), me.GetFullName()) == 0 &&
               controller.GetCallsign() == me.GetCallsign();
    }
}

