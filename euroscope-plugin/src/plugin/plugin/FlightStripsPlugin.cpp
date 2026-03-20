#include <format>
#include "FlightStripsPlugin.h"

#include "Logger.hpp"
#include "bootstrap/Container.h"
#include "graphics/InfoScreen.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "websocket/Events.h"
#include "websocket/WebSocketService.h"

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
        if (!FlightPlan.IsValid()) return;

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
                Logger::Info("Connection type changed: {} -> {}", static_cast<int>(m_connectionState.connection_type), static_cast<int>(connectionType));
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
                    Logger::Info("Callsign changed: '{}' -> '{}'", m_connectionState.callsign, me.GetCallsign());
                    m_connectionState.callsign = {me.GetCallsign()};
                    m_connectionState.relevant_airport = "";
                    // Get relevant airport
                    for (const auto& [airport, prefixes]: m_appConfig->GetCallsignAirportMap()) {
                        for (const auto& prefix: prefixes) {
                            if (_strnicmp(m_connectionState.callsign.c_str(), prefix.c_str(), prefix.length()) == 0) {
                                m_connectionState.relevant_airport = airport;
                                Logger::Info("Found relevant airport: {}", m_connectionState.relevant_airport);
                                break;
                            }
                        }
                        if (!m_connectionState.relevant_airport.empty()) break;
                    }
                    if (m_connectionState.relevant_airport.empty()) {
                        Logger::Warning("No relevant airport found for callsign '{}'", m_connectionState.callsign);
                    }
                }

                const auto primaryFrequency = std::format("{:.3f}", me.GetPrimaryFrequency());
                if (strcmp(primaryFrequency.c_str(), m_connectionState.primary_frequency.c_str()) != 0) {
                    Logger::Info("Primary frequency changed: '{}' -> '{}'", m_connectionState.primary_frequency, primaryFrequency);
                    m_connectionState.primary_frequency = primaryFrequency;
                }

                if (me.GetRange() != m_connectionState.range) {
                    Logger::Debug("Range changed: {} -> {}", m_connectionState.range, me.GetRange());
                    m_connectionState.range = me.GetRange();
                }
            }


            m_timedEventHandlers->OnTimer(Counter);
        });
    }

    void FlightStripsPlugin::OnFlightPlanFlightStripPushed(EuroScopePlugIn::CFlightPlan FlightPlan,
                                                           const char *sSenderController,
                                                           const char *sTargetController) {
        // Facility 4 = Tower
        if (ControllerMyself().GetFacility() != 4) return;

        if (!FlightPlan.IsValid()) return;

        const auto me = ControllerMyself();
        if (_stricmp(FlightPlan.GetHandoffTargetControllerCallsign(), me.GetCallsign()) != 0) return;

        const auto callsign = std::string(FlightPlan.GetCallsign());
        const auto controllerCallsign = std::string(me.GetCallsign());
        SafeCall("OnFlightPlanFlightStripPushed", [this, callsign, controllerCallsign] {
            if (const auto ptr = m_container.lock()) {
                if (ptr->webSocketService->IsConnected()) {
                    ptr->webSocketService->SendEvent(CoordinationReceivedEvent(callsign, controllerCallsign));
                }
            }
        });
    }

    void FlightStripsPlugin::OnRadarTargetPositionUpdate(EuroScopePlugIn::CRadarTarget RadarTarget) {
        if (!RadarTarget.IsValid()) return;

        const auto flightPlan = RadarTarget.GetCorrelatedFlightPlan();

        if (!flightPlan.IsValid()) {
            // No correlated flight plan — VFR/no-FP aircraft. Use radar target position for range check.
            DispatchRangeCheck(RadarTarget);
            return;
        }

        if (IsValidAirports(flightPlan)) {
            SafeCall("OnRadarTargetPositionUpdate", [this, RadarTarget] {
                this->m_radarTargetEventHandlers->RadarTargetPositionEvent(RadarTarget, false);
            });
            return;
        }

        DispatchRangeCheck(RadarTarget);
    }

    void FlightStripsPlugin::DispatchRangeCheck(const CRadarTarget radarTarget) {
        if (IsWithinRange(radarTarget, 30.0f)) {
            SafeCall("OnRadarTargetPositionUpdate", [this, radarTarget] {
                this->m_radarTargetEventHandlers->RadarTargetPositionEvent(radarTarget, true);
            });
        } else {
            // May be out-of-range for a previously range-tracked aircraft — let the service decide
            SafeCall("OnRadarTargetPositionUpdate", [this, radarTarget] {
                this->m_radarTargetEventHandlers->RadarTargetOutOfRangeEvent(radarTarget);
            });
        }
    }

    FlightStripsPlugin::~FlightStripsPlugin() = default;

    bool FlightStripsPlugin::IsValidAirports(const CFlightPlan flightPlan) const {
        return (strcmp(flightPlan.GetFlightPlanData().GetDestination(), m_connectionState.relevant_airport.c_str()) == 0
         || strcmp(flightPlan.GetFlightPlanData().GetOrigin(), m_connectionState.relevant_airport.c_str()) == 0);
    }

    bool FlightStripsPlugin::IsRelevant(const CFlightPlan flightPlan) const {
        if (!flightPlan.IsValid() || flightPlan.GetSimulated() ||
            !flightPlan.GetCorrelatedRadarTarget().IsValid()) {
            return false;
        }
        if (IsValidAirports(flightPlan)) return true;
        // Reject IFR aircraft with a valid flight plan to/from a non-matching airport.
        // Only VFR or no-FP aircraft are eligible for the range-based fallback.
        const auto fpData = flightPlan.GetFlightPlanData();
        const auto origin = std::string(fpData.GetOrigin());
        const auto destination = std::string(fpData.GetDestination());
        if (!origin.empty() && !destination.empty() && origin != "ZZZZ" && destination != "ZZZZ") {
            return false;
        }
        return IsWithinRange(flightPlan.GetCorrelatedRadarTarget(), 30.0f);
    }

    void FlightStripsPlugin::SetAirportCoordinates(const double latitude, const double longitude) {
        m_airportLatitude = latitude;
        m_airportLongitude = longitude;
    }

    bool FlightStripsPlugin::IsWithinRange(const CRadarTarget radarTarget, const float rangeNM) const {
        if (!radarTarget.IsValid()) return false;

        const auto position = radarTarget.GetPosition().GetPosition();

        EuroScopePlugIn::CPosition airportPosition;
        airportPosition.m_Latitude  = m_airportLatitude;
        airportPosition.m_Longitude = m_airportLongitude;

        return position.DistanceTo(airportPosition) <= static_cast<double>(rangeNM);
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
