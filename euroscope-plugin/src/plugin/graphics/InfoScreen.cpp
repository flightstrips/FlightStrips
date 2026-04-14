//
// Created by fsr19 on 11/01/2025.
//

#include "InfoScreen.h"

#include "plugin/FlightStripsPlugin.h"
#include "websocket/Events.h"

namespace FlightStrips::graphics {
    namespace {
        constexpr wchar_t OpenAppUrl[] = L"https://flightstrips.dk/app";

        void SetPreferredSessionMode(const std::shared_ptr<configuration::UserConfig>& userConfig,
                                     FlightStripsPlugin* plugin,
                                     const bool preferSweatbox) {
            auto& state = plugin->GetConnectionState();
            if (IsConnectionSessionForced(state.connection_type) || state.prefer_sweatbox == preferSweatbox) {
                return;
            }

            state.prefer_sweatbox = preferSweatbox;
            userConfig->SetPreferSweatboxSession(preferSweatbox);
        }
    }

    InfoScreen::InfoScreen(const std::shared_ptr<authentication::AuthenticationService>& authenticationService,
                           const std::shared_ptr<configuration::UserConfig>& config,
                           const std::weak_ptr<websocket::WebSocketService>& webSocketService,
                           const std::shared_ptr<flightplan::FlightPlanService>& flightPlanService,
                           const std::shared_ptr<runway::RunwayService>& runwayService,
                           std::shared_ptr<PdcClearancePopupState> pdcPopup,
                           FlightStripsPlugin* plugin)
        : authService(authenticationService),
          userConfig(config),
          webSocketService(webSocketService),
          m_flightPlanService(flightPlanService),
          m_runwayService(runwayService),
          m_pdcPopup(std::move(pdcPopup)),
          m_plugin(plugin) {
        const auto state = userConfig->GetWindowState();
        menubar = {state.x, state.y, state.x + width, state.y + height};
        isMinimized = state.minimized;
    }

    void InfoScreen::OnRefresh(HDC hDC, int Phase) {
        if (Phase != EuroScopePlugIn::REFRESH_PHASE_AFTER_LISTS) {
            return;
        }

        const auto needsSquawk = m_plugin->GetNeedsSquawk();
        if (needsSquawk.has_value()) {
            StartTagFunction("", "TopSky plugin", 0, needsSquawk.value().c_str(), "TopSky plugin", 667, {}, {});
        }

        if (!isOpen) {
            return;
        }

        if (hdcHandle != hDC) {
            hdcHandle = hDC;
            graphics.SetHandle(hdcHandle);
        }

        DrawInfoPanel(*this, graphics, colors, menubar, BuildInfoPanelData(), isMinimized);
        if (isMinimized) {
            canClick = true;
            return;
        }

        if (m_pdcPopup && m_pdcPopup->isOpen) {
            if (const auto popupData = GetPdcPopupData()) {
                DrawPdcPopup(*this, graphics, colors, *m_pdcPopup, *popupData);
            } else {
                m_pdcPopup->isOpen = false;
            }
        }

        canClick = true;
    }

    InfoPanelData InfoScreen::BuildInfoPanelData() const {
        const auto service = webSocketService.lock();

        InfoPanelData data{};
        data.authenticationState = authService->GetAuthenticationState();
        data.authenticatedName = authService->GetName();
        data.connectionState = m_plugin->GetConnectionState();
        data.sessionSelectable = !IsConnectionSessionForced(data.connectionState.connection_type);
        data.effectiveSession = GetEffectiveSessionName(data.connectionState);
        data.connected = service && service->IsConnected();
        data.backingOff = service && service->IsBackingOff();
        data.pending = service && service->IsPendingConnect();
        data.delaySeconds = service ? service->GetDelaySecondsRemaining() : std::nullopt;
        data.stats = service ? service->GetStats() : websocket::Stats{};
        data.showStats = showStats_;
        return data;
    }

    std::optional<PdcPopupData> InfoScreen::GetPdcPopupData() const {
        if (m_pdcPopup == nullptr || !m_pdcPopup->isOpen) {
            return std::nullopt;
        }

        const auto flightPlanService = m_flightPlanService.lock();
        const auto runwayService = m_runwayService.lock();
        return BuildPdcPopupData(*m_pdcPopup,
                                 *m_plugin,
                                 flightPlanService.get(),
                                 runwayService.get());
    }

    void InfoScreen::OnAsrContentToBeClosed() {
        if (hdcHandle) {
            hdcHandle = nullptr;
        }
        delete this;
    }

    void InfoScreen::OnMoveScreenObject(int ObjectType, const char* sObjectId, POINT Pt, RECT Area, bool Released) {
        CRadarScreen::OnMoveScreenObject(ObjectType, sObjectId, Pt, Area, Released);
        if (ObjectType == InfoScreenObjectIds::Window) {
            menubar = Area;

            if (Released) {
                userConfig->SetWindowState({menubar.left, menubar.top, isMinimized});
            }
        } else if (ObjectType == InfoScreenObjectIds::PdcPopupWindow && m_pdcPopup) {
            m_pdcPopup->posX = Area.left;
            m_pdcPopup->posY = Area.top;
            RequestRefresh();
        }
    }

    void InfoScreen::OnClickScreenObject(int ObjectType, const char* sObjectId, POINT Pt, RECT Area, int Button) {
        if (!canClick) {
            return;
        }

        canClick = false;
        if (HandleWindowChromeClick(ObjectType) || HandleSessionModeClick(ObjectType) || HandlePdcPopupClick(ObjectType, Pt, Area)) {
            return;
        }

        if (ObjectType == InfoScreenObjectIds::AuthenticationButton) {
            HandleAuthenticationClick();
            return;
        }

        if (ObjectType == InfoScreenObjectIds::OpenAppButton) {
            HandleOpenAppClick();
            return;
        }

        canClick = true;
    }

    bool InfoScreen::HandleWindowChromeClick(const int objectType) {
        if (objectType == InfoScreenObjectIds::CloseButton) {
            isOpen = false;
            RequestRefresh();
            return true;
        }

        if (objectType == InfoScreenObjectIds::MinimizeButton) {
            isMinimized = !isMinimized;
            userConfig->SetWindowState({menubar.left, menubar.top, isMinimized});
            RequestRefresh();
            return true;
        }

        return false;
    }

    bool InfoScreen::HandleSessionModeClick(const int objectType) {
        if (objectType == InfoScreenObjectIds::SessionLiveButton) {
            SetPreferredSessionMode(userConfig, m_plugin, false);
            if (const auto ws = webSocketService.lock()) {
                ws->Reconnect();
            }
            RequestRefresh();
            return true;
        }

        if (objectType == InfoScreenObjectIds::SessionSweatboxButton) {
            SetPreferredSessionMode(userConfig, m_plugin, true);
            if (const auto ws = webSocketService.lock()) {
                ws->Reconnect();
            }
            RequestRefresh();
            return true;
        }

        if (objectType == InfoScreenObjectIds::StatsToggle) {
            showStats_ = !showStats_;
            RequestRefresh();
            return true;
        }

        return false;
    }

    bool InfoScreen::HandlePdcFieldClick(const int objectType, const POINT pt, const RECT area) {
        if (m_pdcPopup == nullptr) {
            return false;
        }

        if (objectType == InfoScreenObjectIds::PdcFieldRunway) {
            StartTagFunction(m_pdcPopup->callsign.c_str(),
                             PLUGIN_NAME,
                             0,
                             "",
                             nullptr,
                             EuroScopePlugIn::TAG_ITEM_FUNCTION_ASSIGNED_RUNWAY,
                             pt,
                             area);
            return true;
        }

        if (objectType == InfoScreenObjectIds::PdcFieldSid) {
            StartTagFunction(m_pdcPopup->callsign.c_str(),
                             PLUGIN_NAME,
                             0,
                             "",
                             nullptr,
                             EuroScopePlugIn::TAG_ITEM_FUNCTION_ASSIGNED_SID,
                             pt,
                             area);
            return true;
        }

        if (objectType == InfoScreenObjectIds::PdcFieldHeading) {
            StartTagFunction(m_pdcPopup->callsign.c_str(), PLUGIN_NAME, 0, "", "TopSky plugin", 14, pt, area);
            return true;
        }

        if (objectType == InfoScreenObjectIds::PdcFieldCfl) {
            StartTagFunction(m_pdcPopup->callsign.c_str(), PLUGIN_NAME, 0, "", "TopSky plugin", 12, pt, area);
            return true;
        }

        if (objectType == InfoScreenObjectIds::PdcFieldSquawk) {
            StartTagFunction(m_pdcPopup->callsign.c_str(), PLUGIN_NAME, 0, "", "TopSky plugin", 62, pt, area);
            return true;
        }

        if (objectType == InfoScreenObjectIds::PdcFieldRemarks) {
            m_plugin->OpenPopupEdit(area, TAG_FUNC_CLEARANCE_SET_REMARKS, m_pdcPopup->clearanceRemarks.c_str());
            return true;
        }

        return false;
    }

    bool InfoScreen::HandlePdcPopupClick(const int objectType, const POINT pt, const RECT area) {
        if (m_pdcPopup == nullptr || !m_pdcPopup->isOpen) {
            return false;
        }

        if (objectType == InfoScreenObjectIds::PdcBackground) {
            canClick = true;
            return true;
        }

        if (objectType == InfoScreenObjectIds::PdcSendButton) {
            const auto liveFp = m_plugin->FlightPlanSelect(m_pdcPopup->callsign.c_str());
            const bool alreadyClear = liveFp.IsValid() && liveFp.GetClearenceFlag();
            const auto popupData = GetPdcPopupData();

            switch (ResolvePdcPopupPrimaryAction(popupData ? popupData->pdcState : std::string{}, alreadyClear)) {
                case PdcPopupPrimaryAction::IssueRequestedClearance:
                    if (const auto ws = webSocketService.lock()) {
                        ws->SendEvent(IssuePdcClearanceEvent(
                            m_pdcPopup->callsign,
                            popupData ? popupData->clearanceRemarks : std::string{}));
                    }
                    break;
                case PdcPopupPrimaryAction::SetEuroscopeClearance:
                    m_plugin->SetClearenceFlag(m_pdcPopup->callsign, true);
                    break;
                case PdcPopupPrimaryAction::None:
                    break;
            }

            m_pdcPopup->isOpen = false;
            RequestRefresh();
            return true;
        }

        if (objectType == InfoScreenObjectIds::PdcRtButton) {
            const auto popupData = GetPdcPopupData();
            m_pdcPopup->isOpen = false;
            if (popupData && ShouldSendPdcRevertToVoice(popupData->pdcState)) {
                if (const auto ws = webSocketService.lock()) {
                    ws->SendEvent(PdcRevertToVoiceEvent(m_pdcPopup->callsign));
                }
            }
            RequestRefresh();
            return true;
        }

        if (objectType == InfoScreenObjectIds::PdcCancelButton) {
            const auto liveFp = m_plugin->FlightPlanSelect(m_pdcPopup->callsign.c_str());
            if (liveFp.IsValid() && liveFp.GetClearenceFlag()) {
                m_plugin->SetClearenceFlag(m_pdcPopup->callsign, false);
            }
            m_pdcPopup->isOpen = false;
            RequestRefresh();
            return true;
        }

        if (HandlePdcFieldClick(objectType, pt, area)) {
            canClick = true;
            return true;
        }

        return false;
    }

    void InfoScreen::HandleAuthenticationClick() {
        switch (authService->GetAuthenticationState()) {
            case authentication::LOGIN:
                authService->CancelAuthentication();
                break;
            case authentication::REFRESH:
                break;
            case authentication::AUTHENTICATED:
                authService->Logout();
                break;
            case authentication::NONE:
            default:
                authService->StartAuthentication();
                break;
        }

        RequestRefresh();
    }

    void InfoScreen::HandleOpenAppClick() {
        ShellExecute(nullptr, nullptr, OpenAppUrl, nullptr, nullptr, SW_SHOW);
        RequestRefresh();
    }

    bool InfoScreen::OnCompileCommand(const char* sCommandLine) {
        if (_stricmp(sCommandLine, COMMAND_OPEN) == 0) {
            isOpen = true;
            return true;
        }

        if (_stricmp(sCommandLine, COMMAND_CLOSE) == 0) {
            isOpen = false;
            return true;
        }

        if (_stricmp(sCommandLine, COMMAND_CDM_MASTER) == 0) {
            if (const auto ws = webSocketService.lock()) {
                ws->SendEvent(CdmMasterToggleEvent(true));
            }
            return true;
        }

        if (_stricmp(sCommandLine, COMMAND_CDM_SLAVE) == 0) {
            if (const auto ws = webSocketService.lock()) {
                ws->SendEvent(CdmMasterToggleEvent(false));
            }
            return true;
        }

        return false;
    }
}
