#include <format>
#include <cctype>
#include "FlightStripsPlugin.h"

#include "Logger.hpp"
#include "bootstrap/Container.h"
#include "graphics/InfoScreen.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "messages/MessageService.h"
#include "runway/RunwayService.h"
#include "websocket/Events.h"
#include "websocket/WebSocketService.h"

using namespace EuroScopePlugIn;

namespace FlightStrips {
    namespace {
        auto CurrentUtcHHMM() -> std::string {
            time_t rawtime;
            tm ptm;
            time(&rawtime);
            gmtime_s(&ptm, &rawtime);
            return std::format("{:0>2}{:0>2}", ptm.tm_hour, ptm.tm_min);
        }

        bool EqualsIgnoreCase(const std::string& lhs, const char* rhs) {
            return _stricmp(lhs.c_str(), rhs) == 0;
        }
    }

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
        if (const auto container = m_container.lock(); container && container->userConfig) {
            m_connectionState.prefer_sweatbox = container->userConfig->GetPreferSweatboxSession();
        }

        RegisterTagItemType("DE-ICE", TAG_ITEM_DEICING_DESIGNATOR);
        RegisterTagItemType("EOBT", TAG_ITEM_CDM_EOBT);
        RegisterTagItemType("E", TAG_ITEM_CDM_PHASE);
        RegisterTagItemType("TOBT", TAG_ITEM_CDM_TOBT);
        RegisterTagItemType("REQ-TOBT", TAG_ITEM_CDM_REQ_TOBT);
        RegisterTagItemType("TSAT", TAG_ITEM_CDM_TSAT);
        RegisterTagItemType("TSAT/TOBT-DIFF", TAG_ITEM_CDM_TSAT_TOBT_DIFF);
        RegisterTagItemType("TTG", TAG_ITEM_CDM_TTG);
        RegisterTagItemType("TTOT", TAG_ITEM_CDM_TTOT);
        RegisterTagItemType("CTOT", TAG_ITEM_CDM_CTOT);
        RegisterTagItemType("Flow Message", TAG_ITEM_CDM_FLOW_MESSAGE);
        RegisterTagItemType("Network Sts", TAG_ITEM_CDM_NETWORK_STATUS);
        RegisterTagItemType("STATUS", TAG_ITEM_CDM_STATUS);
        RegisterTagItemType("TOBT Confirmed by", TAG_ITEM_CDM_TOBT_CONFIRMED_BY);
        RegisterTagItemType("ASRT", TAG_ITEM_CDM_ASRT);
        RegisterTagItemType("RDY", TAG_ITEM_CDM_READY_STARTUP);
        RegisterTagItemType("TSAC", TAG_ITEM_CDM_TSAC);
        RegisterTagItemType("ASAT", TAG_ITEM_CDM_ASAT);

        RegisterTagItemFunction("Edit EOBT", TAG_FUNC_CDM_EOBT_ACTION);
        RegisterTagItemFunction("EOBT to TOBT", TAG_FUNC_CDM_EOBT_TO_TOBT);
        RegisterTagItemFunction("Edit TOBT", TAG_FUNC_CDM_EDIT_TOBT);
        RegisterTagItemFunction("Ready TOBT", TAG_FUNC_CDM_READY_TOBT);
        RegisterTagItemFunction("TOBT Options", TAG_FUNC_CDM_TOBT_OPTIONS);
        RegisterTagItemFunction("Set TOBT", TAG_FUNC_CDM_SET_TOBT);
        RegisterTagItemFunction("Toggle ASRT", TAG_FUNC_CDM_TOGGLE_ASRT);
        RegisterTagItemFunction("Edit DE-ICE", TAG_FUNC_CDM_EDIT_DEICE);
        RegisterTagItemFunction("DE-ICE Options", TAG_FUNC_CDM_DEICE_OPTIONS);
        RegisterTagItemFunction("Set DE-ICE", TAG_FUNC_CDM_SET_DEICE);
        RegisterTagItemFunction("Edit Manual CTOT", TAG_FUNC_CDM_EDIT_MANUAL_CTOT);
        RegisterTagItemFunction("CTOT Options", TAG_FUNC_CDM_CTOT_OPTIONS);
        RegisterTagItemFunction("Set Manual CTOT", TAG_FUNC_CDM_SET_MANUAL_CTOT);
        RegisterTagItemFunction("Remove Manual CTOT", TAG_FUNC_CDM_REMOVE_MANUAL_CTOT);
        RegisterTagItemFunction("Approve Req TOBT", TAG_FUNC_CDM_APPROVE_REQ_TOBT);
        RegisterTagItemFunction("Options", TAG_FUNC_CDM_OPTIONS);
        RegisterTagItemFunction("CDM Options", TAG_FUNC_CDM_OPTIONS);
        RegisterTagItemFunction("Get FM as text", TAG_FUNC_CDM_FLOW_MESSAGE_AS_TEXT);
        RegisterTagItemFunction("Network Sts Options", TAG_FUNC_CDM_NETWORK_STATUS_OPTIONS);
        RegisterTagItemFunction("Remove DE-ICE", TAG_FUNC_CDM_CLEAR_DEICE);
        RegisterTagItemFunction("Set DE-ICE L", TAG_FUNC_CDM_SET_DEICE_LIGHT);
        RegisterTagItemFunction("Set DE-ICE M", TAG_FUNC_CDM_SET_DEICE_MEDIUM);
        RegisterTagItemFunction("Set DE-ICE H", TAG_FUNC_CDM_SET_DEICE_HEAVY);
        RegisterTagItemFunction("Set DE-ICE J", TAG_FUNC_CDM_SET_DEICE_JUMBO);
        RegisterTagItemFunction("TSAC Options", TAG_FUNC_CDM_TSAC_OPTIONS);
        RegisterTagItemFunction("Add TSAT to TSAC", TAG_FUNC_CDM_TOGGLE_TSAC);
        RegisterTagItemFunction("Remove TSAC", TAG_FUNC_CDM_TOGGLE_TSAC);
        RegisterTagItemFunction("Edit TSAC", TAG_FUNC_CDM_EDIT_TSAC);
        RegisterTagItemFunction("Edit Clearance Remarks", TAG_FUNC_CLEARANCE_EDIT_REMARKS);
        RegisterTagItemFunction("Set Clearance Remarks", TAG_FUNC_CLEARANCE_SET_REMARKS);

        RegisterTagItemType("CLR / PDC", TAG_ITEM_CLEARANCE_STATUS);
        RegisterTagItemFunction("CLR / PDC Options", TAG_FUNC_CLEARANCE_OPTIONS);
    }

    void FlightStripsPlugin::Information(const std::string &message) {
        DisplayUserMessage("FlightStrips", PLUGIN_NAME, message.c_str(), true, false, false, false, false);
    }

    void FlightStripsPlugin::Information(const char *message) {
        DisplayUserMessage("FlightStrips", PLUGIN_NAME, message, true, false, false, false, false);
    }

    void FlightStripsPlugin::Error(const std::string &message) {
        DisplayUserMessage("FlightStrips", PLUGIN_NAME, message.c_str(), true, true, true, true, true);
    }

    void FlightStripsPlugin::Error(const char *message) {
        DisplayUserMessage("FlightStrips", PLUGIN_NAME, message, true, true, true, true, true);
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

        return exceptions::RunGuardedOr<CRadarScreen*>("OnRadarScreenCreated", nullptr, [this]() -> CRadarScreen* {
            if (const auto ptr = m_container.lock()) {
                return new graphics::InfoScreen(
                    ptr->authenticationService,
                    ptr->userConfig,
                    ptr->webSocketService,
                    ptr->flightPlanService,
                    ptr->runwayService,
                    ptr->pdcPopup,
                    this);
            }

            return nullptr;
        }, [this](const exceptions::ExceptionDetails&) noexcept {
            Error("FlightStrips failed to create the radar screen. See the log for details.");
        });
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

    void FlightStripsPlugin::OnFunctionCall(int FunctionId, const char* ItemString, POINT Pt, RECT Area) {
        const auto container = m_container.lock();
        if (container == nullptr || container->messageService == nullptr) return;

        const auto fp = FlightPlanSelectASEL();
        auto callsign = fp.IsValid() ? std::string(fp.GetCallsign()) : std::string{};
        if (callsign.empty() &&
            (FunctionId == TAG_FUNC_CLEARANCE_EDIT_REMARKS || FunctionId == TAG_FUNC_CLEARANCE_SET_REMARKS) &&
            container->pdcPopup != nullptr) {
            callsign = container->pdcPopup->callsign;
        }
        if (callsign.empty()) return;

        const auto tracked = container->flightPlanService->GetFlightPlan(callsign);
        const auto currentEobt = tracked == nullptr ? "" : tracked->cdm.eobt;
        const auto currentTobt = tracked == nullptr ? "" : tracked->cdm.tobt;
        const auto currentReqTobt = tracked == nullptr ? "" : tracked->cdm.req_tobt;
        const auto currentManualCtot = tracked == nullptr ? "" : tracked->cdm.manual_ctot;
        const auto currentAsrt = tracked == nullptr ? "" : tracked->cdm.asrt;
        const auto currentTsat = tracked == nullptr ? "" : tracked->cdm.tsat;
        const auto currentTsac = tracked == nullptr ? "" : tracked->cdm.tsac;
        const auto currentDeice = tracked == nullptr ? "" : tracked->cdm.deice_type;
        const auto currentFlowMessage = tracked == nullptr ? "" : tracked->cdm.ecfmp_id;

        const auto addEobtActions = [&] {
            if (IsValidHhmm(currentEobt)) {
                AddPopupListElement("Copy EOBT to TOBT", "", TAG_FUNC_CDM_EOBT_TO_TOBT, false, 2, false);
            }
            AddPopupListElement("Edit TOBT", "", TAG_FUNC_CDM_EDIT_TOBT, false, 2, false);
        };

        const auto addTobtOptions = [&] {
            AddPopupListElement("Ready TOBT", "", TAG_FUNC_CDM_READY_TOBT, false, 2, false);
            AddPopupListElement("Edit TOBT", "", TAG_FUNC_CDM_EDIT_TOBT, false, 2, false);
            if (!currentReqTobt.empty()) {
                AddPopupListElement("Approve Req TOBT", "", TAG_FUNC_CDM_APPROVE_REQ_TOBT, false, 2, false);
            }
        };

        const auto addCtotOptions = [&] {
            AddPopupListElement(currentManualCtot.empty() ? "Set Manual CTOT" : "Edit Manual CTOT", "", TAG_FUNC_CDM_EDIT_MANUAL_CTOT, false, 2, false);
            if (!currentManualCtot.empty()) {
                AddPopupListElement("Remove Manual CTOT", "", TAG_FUNC_CDM_REMOVE_MANUAL_CTOT, false, 2, false);
            }
        };

        const auto addDeiceOptions = [&] {
            AddPopupListElement("Remove DE-ICE", "", TAG_FUNC_CDM_CLEAR_DEICE, false, 2, false);
            AddPopupListElement("Set DE-ICE L", "", TAG_FUNC_CDM_SET_DEICE_LIGHT, false, 2, false);
            AddPopupListElement("Set DE-ICE M", "", TAG_FUNC_CDM_SET_DEICE_MEDIUM, false, 2, false);
            AddPopupListElement("Set DE-ICE H", "", TAG_FUNC_CDM_SET_DEICE_HEAVY, false, 2, false);
            AddPopupListElement("Set DE-ICE J", "", TAG_FUNC_CDM_SET_DEICE_JUMBO, false, 2, false);
        };

        const auto addTsacOptions = [&] {
            AddPopupListElement(currentTsac.empty() ? "Add TSAT to TSAC" : "Remove TSAC", "", TAG_FUNC_CDM_TOGGLE_TSAC, false, 2, false);
            AddPopupListElement("Edit TSAC", "", TAG_FUNC_CDM_EDIT_TSAC, false, 2, false);
        };

        const auto addOptionsSeparator = [&] {
            AddPopupListElement("---", "", TAG_ITEM_FUNCTION_NO, false, 2, true);
        };

        const auto openEobtActions = [&] {
            OpenPopupList(Area, "EOBT Actions", 1);
            addEobtActions();
        };

        const auto openTobtOptions = [&] {
            OpenPopupList(Area, "TOBT Options", 1);
            addTobtOptions();
        };

        const auto openCtotOptions = [&] {
            OpenPopupList(Area, "CTOT Options", 1);
            addCtotOptions();
        };

        const auto openDeiceOptions = [&] {
            OpenPopupList(Area, "DE-ICE Options", 1);
            addDeiceOptions();
        };

        const auto openTsacOptions = [&] {
            OpenPopupList(Area, "TSAC Options", 1);
            addTsacOptions();
        };

        const auto openGlobalOptions = [&] {
            OpenPopupList(Area, "CDM Options", 1);
            addEobtActions();
            addOptionsSeparator();
            addTobtOptions();
            addOptionsSeparator();
            addCtotOptions();
            addOptionsSeparator();
            addTsacOptions();
            addOptionsSeparator();
            addDeiceOptions();
            addOptionsSeparator();
            AddPopupListElement(currentAsrt.empty() ? "Set ASRT" : "Clear ASRT", "", TAG_FUNC_CDM_TOGGLE_ASRT, false, 2, false);
        };

        const auto openClearanceDialog = [&] {
            if (container->pdcPopup == nullptr) {
                return false;
            }

            auto& popup = *container->pdcPopup;
            popup.callsign = callsign;
            popup.clearanceRemarks.clear();
            popup.posX = Pt.x;
            popup.posY = Pt.y;
            popup.isOpen = true;
            return true;
        };

        switch (FunctionId) {
            case TAG_FUNC_CDM_EOBT_ACTION:
                openEobtActions();
                break;
            case TAG_FUNC_CDM_EOBT_TO_TOBT:
                if (IsValidHhmm(currentEobt)) {
                    container->messageService->SendCdmTobtUpdate(callsign, currentEobt);
                }
                break;
            case TAG_FUNC_CDM_EDIT_TOBT:
                OpenPopupEdit(Area, TAG_FUNC_CDM_SET_TOBT, currentTobt.c_str());
                break;
            case TAG_FUNC_CDM_READY_TOBT:
                container->messageService->SendCdmTobtUpdate(callsign, CurrentUtcHHMM());
                break;
            case TAG_FUNC_CDM_TOBT_OPTIONS:
                openTobtOptions();
                break;
            case TAG_FUNC_CDM_SET_TOBT:
                if (ItemString != nullptr && IsValidHhmm(ItemString)) {
                    container->messageService->SendCdmTobtUpdate(callsign, ItemString);
                }
                break;
            case TAG_FUNC_CDM_TOGGLE_ASRT: {
                if (currentAsrt.empty()) {
                    container->messageService->SendCdmAsrtToggle(callsign, CurrentUtcHHMM());
                } else {
                    container->messageService->SendCdmAsrtToggle(callsign, "");
                }
                break;
            }
            case TAG_FUNC_CDM_TSAC_OPTIONS:
                openTsacOptions();
                break;
            case TAG_FUNC_CDM_TOGGLE_TSAC:
                if (currentTsac.empty()) {
                    if (IsValidHhmm(currentTsat)) {
                        container->messageService->SendCdmTsacUpdate(callsign, currentTsat);
                    }
                } else {
                    container->messageService->SendCdmTsacUpdate(callsign, "");
                }
                break;
            case TAG_FUNC_CDM_EDIT_TSAC:
                OpenPopupEdit(Area, TAG_FUNC_CDM_SET_TSAC, currentTsac.c_str());
                break;
            case TAG_FUNC_CDM_SET_TSAC:
                if (ItemString != nullptr) {
                    const auto value = std::string(ItemString);
                    if (value.empty() || IsValidHhmm(value)) {
                        container->messageService->SendCdmTsacUpdate(callsign, value);
                    }
                }
                break;
            case TAG_FUNC_CDM_EDIT_DEICE:
                OpenPopupEdit(Area, TAG_FUNC_CDM_SET_DEICE, currentDeice.c_str());
                break;
            case TAG_FUNC_CDM_DEICE_OPTIONS:
                openDeiceOptions();
                break;
            case TAG_FUNC_CDM_CLEAR_DEICE:
                container->messageService->SendCdmDeiceUpdate(callsign, "");
                break;
            case TAG_FUNC_CDM_SET_DEICE_LIGHT:
                container->messageService->SendCdmDeiceUpdate(callsign, "L");
                break;
            case TAG_FUNC_CDM_SET_DEICE_MEDIUM:
                container->messageService->SendCdmDeiceUpdate(callsign, "M");
                break;
            case TAG_FUNC_CDM_SET_DEICE_HEAVY:
                container->messageService->SendCdmDeiceUpdate(callsign, "H");
                break;
            case TAG_FUNC_CDM_SET_DEICE_JUMBO:
                container->messageService->SendCdmDeiceUpdate(callsign, "J");
                break;
            case TAG_FUNC_CDM_SET_DEICE: {
                const auto value = ItemString == nullptr ? std::string{} : std::string(ItemString);
                if (value.empty() || value == "L" || value == "M" || value == "H" || value == "J") {
                    container->messageService->SendCdmDeiceUpdate(callsign, value);
                }
                break;
            }
            case TAG_FUNC_CDM_EDIT_MANUAL_CTOT:
                OpenPopupEdit(Area, TAG_FUNC_CDM_SET_MANUAL_CTOT, currentManualCtot.c_str());
                break;
            case TAG_FUNC_CDM_CTOT_OPTIONS:
                openCtotOptions();
                break;
            case TAG_FUNC_CDM_SET_MANUAL_CTOT:
                if (ItemString != nullptr && IsValidHhmm(ItemString)) {
                    container->messageService->SendCdmManualCtot(callsign, ItemString);
                }
                break;
            case TAG_FUNC_CDM_REMOVE_MANUAL_CTOT:
                container->messageService->SendCdmCtotRemove(callsign);
                break;
            case TAG_FUNC_CDM_APPROVE_REQ_TOBT:
                if (!currentReqTobt.empty()) {
                    container->messageService->SendCdmApproveReqTobt(callsign);
                }
                break;
            case TAG_FUNC_CDM_OPTIONS:
                openGlobalOptions();
                break;
            case TAG_FUNC_CLEARANCE_OPTIONS: {
                openClearanceDialog();
                break;
            }
            case TAG_FUNC_CLEARANCE_EDIT_REMARKS:
                if (container->pdcPopup != nullptr) {
                    OpenPopupEdit(Area, TAG_FUNC_CLEARANCE_SET_REMARKS, container->pdcPopup->clearanceRemarks.c_str());
                }
                break;
            case TAG_FUNC_CLEARANCE_SET_REMARKS:
                if (container->pdcPopup != nullptr && container->pdcPopup->callsign == callsign) {
                    container->pdcPopup->clearanceRemarks = ItemString == nullptr ? std::string{} : std::string(ItemString);
                }
                break;
            case TAG_FUNC_CDM_FLOW_MESSAGE_AS_TEXT:
                Information(currentFlowMessage.empty() ? "No flow message available." : currentFlowMessage);
                break;
            case TAG_FUNC_CDM_NETWORK_STATUS_OPTIONS:
                if (!currentReqTobt.empty()) {
                    OpenPopupList(Area, "Network Sts Options", 1);
                    AddPopupListElement("Approve Req TOBT", "", TAG_FUNC_CDM_APPROVE_REQ_TOBT, false, 2, false);
                } else {
                    Information("Network status is backend-driven.");
                }
                break;
            default:
                break;
        }
    }

    bool FlightStripsPlugin::ControllerIsMe(EuroScopePlugIn::CController controller, EuroScopePlugIn::CController me) {
        return controller.IsValid() && strcmp(controller.GetFullName(), me.GetFullName()) == 0 &&
               controller.GetCallsign() == me.GetCallsign();
    }

    bool FlightStripsPlugin::IsValidHhmm(const std::string& value) {
        if (value.size() != 4) return false;
        for (const char c : value) {
            if (!std::isdigit(static_cast<unsigned char>(c))) return false;
        }
        const auto hour = std::stoi(value.substr(0, 2));
        const auto minute = std::stoi(value.substr(2, 2));
        return hour >= 0 && hour < 24 && minute >= 0 && minute < 60;
    }
}
