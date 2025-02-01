#include <format>
#include "FlightStripsPlugin.h"

#include "Logger.h"
#include "graphics/InfoScreen.h"
#include "handlers/FlightPlanEventHandlers.h"

using namespace EuroScopePlugIn;

namespace FlightStrips {
    FlightStripsPlugin::FlightStripsPlugin(
        const std::shared_ptr<handlers::FlightPlanEventHandlers> &mFlightPlanEventHandlerCollection,
        const std::shared_ptr<handlers::RadarTargetEventHandlers> &mRadarTargetEventHandlers,
        const std::shared_ptr<handlers::ControllerEventHandlers> &mControllerEventHandlers,
        const std::shared_ptr<handlers::TimedEventHandlers> &mTimedEventHandlers,
        const std::shared_ptr<handlers::AirportRunwaysChangedEventHandlers> &mAirportRunwaysChangedEventHandlers,
        const std::shared_ptr<authentication::AuthenticationService> &mAuthenticationService,
        const std::shared_ptr<configuration::UserConfig> &mUserConfig,
        const std::shared_ptr<configuration::AppConfig> &mAppConfig)
        : CPlugIn(COMPATIBILITY_CODE, PLUGIN_NAME, PLUGIN_VERSION, PLUGIN_AUTHOR, PLUGIN_COPYRIGHT),
          m_flightPlanEventHandlerCollection(mFlightPlanEventHandlerCollection),
          m_radarTargetEventHandlers(mRadarTargetEventHandlers),
          m_controllerEventHandlerCollection(mControllerEventHandlers),
          m_timedEventHandlers(mTimedEventHandlers),
          m_airportRunwayChangedEventHandlers(mAirportRunwaysChangedEventHandlers),
          m_authenticationService(mAuthenticationService),
          m_userConfig(mUserConfig),
          m_appConfig(mAppConfig) {
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
        const auto connectionType = static_cast<ConnectionType>(GetConnectionType());

        if (m_connectionState.connection_type != connectionType) {
            Logger::Debug("Connection type change to: {}", static_cast<int>(connectionType));
            m_connectionState.callsign = "";
            m_connectionState.primary_frequency = "";
            m_connectionState.range = 0;
            m_connectionState.relevant_airport = "";
            m_connectionState.connection_type = connectionType;
        }

        if (m_connectionState.connection_type == CONNECTION_TYPE_DIRECT || m_connectionState.connection_type ==
            CONNECTION_TYPE_PLAYBACK || m_connectionState.connection_type == CONNECTION_TYPE_SWEATBOX) {
            const auto me = ControllerMyself();

            if (strcmp(me.GetCallsign(), m_connectionState.callsign.c_str()) != 0) {
                m_connectionState.callsign = {me.GetCallsign()};
                Logger::Debug("Setting callsign: {}", m_connectionState.callsign);
                m_connectionState.relevant_airport = "";
                // Get relevant airport
                for (const auto& [airport, prefixes]: m_appConfig->GetCallsignAirportMap()) {
                    for (const auto& prefix: prefixes) {
                        if (_strnicmp(m_connectionState.callsign.c_str(), prefix.c_str(), prefix.length()) == 0) {
                            m_connectionState.relevant_airport = airport;
                            Logger::Debug("Found relevant airport: {}", m_connectionState.relevant_airport);
                            break;
                        }
                    }
                    if (!m_connectionState.relevant_airport.empty()) break;
                }
            }

            const auto primaryFrequency = std::format("{:.3f}", me.GetPrimaryFrequency());
            if (strcmp(primaryFrequency.c_str(), m_connectionState.primary_frequency.c_str()) != 0) {
                m_connectionState.primary_frequency = primaryFrequency;
                Logger::Debug("Setting primary frequency: {}", m_connectionState.primary_frequency);
            }

            if (me.GetRange() != m_connectionState.range) {
                m_connectionState.range = me.GetRange();
                Logger::Debug("Setting range: {}", m_connectionState.range);
            }
        }


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

    ConnectionState &FlightStripsPlugin::GetConnectionState() {
        return m_connectionState;
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


    CRadarScreen *FlightStripsPlugin::OnRadarScreenCreated(const char *sDisplayName,
                                                           bool NeedRadarContent, bool GeoReferenced, bool CanBeSaved,
                                                           bool CanBeCreated) {
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
