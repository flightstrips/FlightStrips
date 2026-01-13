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
        const std::weak_ptr<Container> &mContainer,
        const std::shared_ptr<configuration::AppConfig> &mAppConfig,
        const std::shared_ptr<TagItems::TagItemHandlers> &mTagItemHandlers)
        : CPlugIn(COMPATIBILITY_CODE, PLUGIN_NAME, PLUGIN_VERSION, PLUGIN_AUTHOR, PLUGIN_COPYRIGHT),
          m_flightPlanEventHandlerCollection(mFlightPlanEventHandlerCollection),
          m_radarTargetEventHandlers(mRadarTargetEventHandlers),
          m_controllerEventHandlerCollection(mControllerEventHandlers),
          m_timedEventHandlers(mTimedEventHandlers),
          m_airportRunwayChangedEventHandlers(mAirportRunwaysChangedEventHandlers),
          m_appConfig(mAppConfig),
          m_tagItemHandlers(mTagItemHandlers),
          m_container(mContainer) {
        RegisterTagItemType("DE-ICE", TAG_ITEM_DEICING_DESIGNATOR);
    }

    void FlightStripsPlugin::Information(const std::string &message) {
        DisplayUserMessage("FlightStrips", PLUGIN_NAME, message.c_str(), true, false, false, false, false);
    }

    void FlightStripsPlugin::Error(const std::string &message) {
        DisplayUserMessage("FlightStrips", PLUGIN_NAME, message.c_str(), true, true, true, true, true);
    }

    void FlightStripsPlugin::OnFlightPlanDisconnect(EuroScopePlugIn::CFlightPlan FlightPlan) {
        if (!IsValidAirports(FlightPlan) || !FlightPlan.IsValid()) {
            return;
        }

        SafeCall("OnFlightPlanDisconnect", [this, FlightPlan] {
            this->m_flightPlanEventHandlerCollection->FlightPlanDisconnectEvent(FlightPlan);
        });
    }

    void FlightStripsPlugin::OnFlightPlanControllerAssignedDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan,
                                                                      int DataType) {
        if (!IsRelevant(FlightPlan)) {
            return;
        }

        SafeCall("OnFlightPlanControllerAssignedDataUpdate", [this, FlightPlan, DataType] {
            this->m_flightPlanEventHandlerCollection->ControllerFlightPlanDataEvent(FlightPlan, DataType);
        });
    }

    void FlightStripsPlugin::OnFlightPlanFlightPlanDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan) {
        if (!IsRelevant(FlightPlan)) {
            return;
        }

        SafeCall("OnFlightPlanFlightPlanDataUpdate", [this, FlightPlan] {
            this->m_flightPlanEventHandlerCollection->FlightPlanEvent(FlightPlan);
        });
    }

    void FlightStripsPlugin::OnTimer(int Counter) {
        SafeCall("OnTimer", [this, Counter] {
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


            m_timedEventHandlers->OnTimer(Counter);
        });
    }

    void FlightStripsPlugin::OnFlightPlanFlightStripPushed(EuroScopePlugIn::CFlightPlan, const char *, const char *) {
    }

    void FlightStripsPlugin::OnRadarTargetPositionUpdate(EuroScopePlugIn::CRadarTarget RadarTarget) {
        if (const auto flightPlan = RadarTarget.GetCorrelatedFlightPlan(); !RadarTarget.IsValid() || !flightPlan.IsValid() || !IsValidAirports(flightPlan)) {
            return;
        }

        SafeCall("OnRadarTargetPositionUpdate", [this, RadarTarget] {
            this->m_radarTargetEventHandlers->RadarTargetPositionEvent(RadarTarget);
        });
    }

    FlightStripsPlugin::~FlightStripsPlugin() = default;

    bool FlightStripsPlugin::IsValidAirports(const CFlightPlan flightPlan) const {
        return (strcmp(flightPlan.GetFlightPlanData().GetDestination(), m_connectionState.relevant_airport.c_str()) == 0
         || strcmp(flightPlan.GetFlightPlanData().GetOrigin(), m_connectionState.relevant_airport.c_str()) == 0);
    }

    bool FlightStripsPlugin::IsRelevant(const CFlightPlan flightPlan) const {
        return flightPlan.IsValid() && !flightPlan.GetSimulated() && flightPlan.GetCorrelatedRadarTarget().IsValid() &&
            IsValidAirports(flightPlan);
    }

    ConnectionState &FlightStripsPlugin::GetConnectionState() {
        return m_connectionState;
    }

    std::vector<Sid> FlightStripsPlugin::GetSids(const std::string& airport) {
        // Assumption name of the element will be in the format of: <airport-fixed-length> SID <runway> <sid>
        constexpr size_t start = std::string_view("EKCH SID ").length();
        std::vector<Sid> sids;

        for (auto it = SectorFileElementSelectFirst(SECTOR_ELEMENT_SID); it.IsValid(); it = SectorFileElementSelectNext(it, SECTOR_ELEMENT_SID)) {
            const auto sid= it.GetName();
            if (_strnicmp(sid, airport.c_str(), 4) != 0) continue;
            auto str = std::string(sid);

            const auto pos = str.find(' ', start);
            const auto runway = str.substr(start, pos - start);
            const auto name = str.substr(pos + 1);

            sids.emplace_back(name, runway);
        }

        return sids;
    }

    void FlightStripsPlugin::AddNeedsSquawk(const std::string &callsign) {
        m_needsSquawk.push(callsign);
    }

    std::optional<std::string> FlightStripsPlugin::GetNeedsSquawk() {
        if (m_needsSquawk.empty()) {
            return {};
        }

        auto needsSquawk = m_needsSquawk.front();
        m_needsSquawk.pop();
        return needsSquawk;
    }

    void FlightStripsPlugin::OnAirportRunwayActivityChanged() {
        SafeCall("OnAirportRunwayActivityChanged", [this] {
            m_airportRunwayChangedEventHandlers->OnAirportRunwayActivityChanged();
        });
    }

    void FlightStripsPlugin::SetClearenceFlag(const std::string &callsign, const bool cleared) const {
        if (cleared) {
            this->UpdateViaScratchPad(callsign.c_str(), CLEARED);
        } else {
            this->UpdateViaScratchPad(callsign.c_str(), NOT_CLEARED);
        }
    }

    void FlightStripsPlugin::SetArrivalStand(const std::string &callsign, std::string stand) const {
        UpdateViaScratchPad(callsign.c_str(), std::format("GRP/S/{}", stand).c_str());
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
        if (!m_appConfig->GetApiEnabled()) {
            return nullptr;
        }
        if (const auto ptr = m_container.lock()) {
            return new graphics::InfoScreen(ptr->authenticationService, ptr->userConfig, ptr->webSocketService, this);
        }

        return nullptr;
    }

    void FlightStripsPlugin::OnControllerPositionUpdate(EuroScopePlugIn::CController Controller) {
        if (!Controller.IsValid()) return;
        if (!Controller.GetPositionIdentified()) return;
        if (!Controller.IsController()) return;

        SafeCall("OnControllerPositionUpdate", [this, Controller] {
            this->m_controllerEventHandlerCollection->ControllerPositionUpdateEvent(Controller);
        });
    }

    void FlightStripsPlugin::OnControllerDisconnect(EuroScopePlugIn::CController Controller) {
        SafeCall("OnControllerDisconnect", [this, Controller] {
            this->m_controllerEventHandlerCollection->ControllerDisconnectEvent(Controller);
        });
    }

    void FlightStripsPlugin::OnGetTagItem(EuroScopePlugIn::CFlightPlan FlightPlan,
        EuroScopePlugIn::CRadarTarget RadarTarget, int ItemCode, int TagData, char sItemString[16], int *pColorCode,
        COLORREF *pRGB, double *pFontSize) {
        if (!FlightPlan.IsValid()) return;

        SafeCall("OnGetTagItem", [this, FlightPlan, RadarTarget, ItemCode, TagData, sItemString, pColorCode, pRGB, pFontSize] {
            m_tagItemHandlers->Handle(FlightPlan, RadarTarget, ItemCode, TagData, sItemString, pColorCode, pRGB, pFontSize);
        });
    }

    bool FlightStripsPlugin::ControllerIsMe(EuroScopePlugIn::CController controller, EuroScopePlugIn::CController me) {
        return controller.IsValid() && strcmp(controller.GetFullName(), me.GetFullName()) == 0 &&
               controller.GetCallsign() == me.GetCallsign();
    }
}
