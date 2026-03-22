//
// Created by fsr19 on 11/01/2025.
//

#include "InfoScreen.h"

#include "Colors.h"
#include "plugin/FlightStripsPlugin.h"

namespace FlightStrips::graphics {
    namespace {
        std::string FormatCount(const int n) {
            if (n < 1000) return std::to_string(n);
            if (n < 1000000) return std::format("{:.1f}K", n / 1000.0);
            return std::format("{:.1f}M", n / 1000000.0);
        }
    }
    InfoScreen::InfoScreen(
        const std::shared_ptr<authentication::AuthenticationService> &authenticationService,
        const std::shared_ptr<configuration::UserConfig> &config,
        const std::weak_ptr<websocket::WebSocketService> &webSocketService,
        FlightStripsPlugin *plugin) : authService(
                                          authenticationService),
                                      userConfig(config),
                                      webSocketService(webSocketService),
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
        const auto needsCdmReady = m_plugin->GetNeedsCdmReady();
        if (needsCdmReady.has_value()) {
            cdmReadyTrigger.Trigger(*this, needsCdmReady.value());
        }

        if (!isOpen) {
            return;
        }

        if (hdcHandle != hDC) {
            hdcHandle = hDC;
            graphics.SetHandle(hdcHandle);
        }

        AddScreenObject(windowId, "", menubar, true, nullptr);

        const RECT closeBth = {menubar.right - 12, menubar.top + 3, menubar.right - 3, menubar.bottom - 3};
        AddScreenObject(closeId, "", closeBth, false, nullptr);
        const RECT minimizeBtn = {menubar.right - 30, menubar.top + 3, menubar.right - 18, menubar.bottom - 3};
        AddScreenObject(minimizeId, "", minimizeBtn, false, nullptr);

        // Compute state early so window background can be drawn before the header
        const auto service = webSocketService.lock();
        const bool connected = service && service->IsConnected();
        const bool pending   = service && service->IsPendingConnect();
        const auto delay     = service ? service->GetDelaySecondsRemaining() : std::nullopt;
        const auto stats     = service ? service->GetStats() : websocket::Stats{};
        const auto& cs       = m_plugin->GetConnectionState();

        // Dynamic window height
        // Base: account(58) + sep-gap(9) + status-row(16) = 83
        // Always: padding(3) + sep(8) + toggle(14) = 25
        // Stats open: 3 info rows (13 each) = 39 (if connected) + TX/RX/Q (3+13) = 16
        // Bottom padding: 5
        int contentH = 83 + 25 + 5;
        if (showStats_) {
            if (connected) contentH += 39;
            contentH += 16;
        }

        // Draw background first, then header on top so it is never overwritten
        if (!isMinimized) {
            const RECT windowRect = {menubar.left, menubar.top, menubar.right, menubar.bottom + contentH};
            graphics.FillRect(colors.backgroundBrush.get(), windowRect);
            graphics.DrawRect(colors.backgroundPen.get(), windowRect);
        }

        const Gdiplus::Brush* dotBrush = connected ? colors.greenBrush.get()
                                       : pending   ? colors.orangeBrush.get()
                                                   : colors.redBrush.get();

        graphics.FillRect(colors.headerBrush.get(), menubar);
        graphics.DrawString("FlightStrips", menubar, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
        graphics.DrawXButton(colors.buttonPen.get(), closeBth);
        graphics.DrawLineButton(colors.buttonPen.get(), minimizeBtn);
        const RECT headerDot = {menubar.right - 44, menubar.top + 4, menubar.right - 36, menubar.top + 12};
        graphics.FillEllipse(dotBrush, headerDot);

        if (isMinimized) {
            canClick = true;
            return;
        }

        const int L = menubar.left + 5;
        const int R = menubar.right - 5;
        int y = menubar.bottom;

        // ── Account ──────────────────────────────────────────────
        const RECT authTextRect = {L, y + 4, R, y + 34};
        std::string btnText;
        switch (authService->GetAuthenticationState()) {
            case authentication::LOGIN:
                graphics.DrawString("Logging in...", authTextRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                btnText = "Cancel";
                break;
            case authentication::REFRESH:
                graphics.DrawString("Refreshing token...", authTextRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                btnText = "No action";
                break;
            case authentication::AUTHENTICATED:
                graphics.DrawString(std::format("Logged in as:\n{}", authService->GetName()), authTextRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                btnText = "Logout";
                break;
            case authentication::NONE:
            default:
                graphics.DrawString("Logged out.", authTextRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                btnText = "Login";
                break;
        }

        const RECT btnRect = {L + 5, y + 38, R - 70, y + 54};
        graphics.FillRect(colors.headerBrush.get(), btnRect);
        graphics.DrawString(btnText, btnRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentCenter);
        AddScreenObject(authenticationButtonId, "", btnRect, false, nullptr);

        y += 58;

        // ── Separator ─────────────────────────────────────────────
        graphics.DrawHLine(colors.separatorPen.get(), menubar.left + 3, y + 4, menubar.right - 3);
        y += 9;

        // ── Connection status ─────────────────────────────────────
        const std::string statusText = connected ? "Connected"
                                     : pending   ? (delay.has_value() ? std::format("Syncing  ({}s)", delay.value()) : "Connecting...")
                                                 : "Disconnected";

        const RECT dotRect = {L, y + 4, L + 8, y + 12};
        graphics.FillEllipse(dotBrush, dotRect);
        const RECT statusLabelRect = {L + 13, y, R, y + 16};
        graphics.DrawString(statusText, statusLabelRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
        y += 16;

        y += 3;

        // ── Separator ─────────────────────────────────────────────
        graphics.DrawHLine(colors.separatorPen.get(), menubar.left + 3, y + 3, menubar.right - 3);
        y += 8;

        // ── Stats toggle ──────────────────────────────────────────
        const RECT statsToggleRect = {L, y, R, y + 14};
        AddScreenObject(statsToggleId, "", statsToggleRect, false, nullptr);
        graphics.DrawString(showStats_ ? "Stats  [-]" : "Stats  [+]", statsToggleRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
        y += 14;

        // ── Stats content ─────────────────────────────────────────
        if (showStats_ && service) {
            if (connected) {
                const std::string connType = cs.connection_type == CONNECTION_TYPE_DIRECT   ? "LIVE"
                                           : cs.connection_type == CONNECTION_TYPE_SWEATBOX ? "SWX"
                                                                                            : "PBK";
                const std::string roleStr  = stats.role == websocket::STATE_MASTER ? "MASTER"
                                           : stats.role == websocket::STATE_SLAVE  ? "SLAVE"
                                                                                   : "SYNC";

                const RECT infoRow1 = {L + 2, y, R, y + 13};
                graphics.DrawString(cs.callsign, infoRow1, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                y += 13;

                const RECT infoRow2 = {L + 2, y, R, y + 13};
                graphics.DrawString(std::format("{}  {}", cs.primary_frequency, cs.relevant_airport), infoRow2, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                y += 13;

                const RECT infoRow3 = {L + 2, y, R, y + 13};
                graphics.DrawString(std::format("{}  {}", roleStr, connType), infoRow3, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                y += 13;
            }

            y += 3;
            const RECT statsRect = {L + 2, y, R, y + 13};
            graphics.DrawString(
                std::format("TX {}   RX {}   Q {}", FormatCount(stats.tx), FormatCount(stats.rx), FormatCount(stats.queued)),
                statsRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear
            );
        }

        canClick = true;
    }

    void InfoScreen::OnAsrContentToBeClosed() {
        if (hdcHandle) {
            hdcHandle = nullptr;
        }
        delete this;
    }

    bool InfoScreen::SelectActiveAircraft(const std::string& callsign) {
        const auto flightPlan = m_plugin->FlightPlanSelect(callsign.c_str());
        if (!flightPlan.IsValid()) {
            return false;
        }

        m_plugin->SetASELAircraft(flightPlan);
        return true;
    }

    void InfoScreen::InvokeTagFunction(
        const std::string& itemString,
        const std::string& menuName,
        const int menuItemCode,
        const std::string& parameter,
        const std::string& targetPluginName,
        const int targetFunctionId) {
        CRadarScreen::StartTagFunction(
            itemString.c_str(),
            menuName.c_str(),
            menuItemCode,
            parameter.c_str(),
            targetPluginName.c_str(),
            targetFunctionId,
            {},
            {});
    }

    void InfoScreen::OnMoveScreenObject(int ObjectType, const char *sObjectId, POINT Pt, RECT Area, bool Released) {
        CRadarScreen::OnMoveScreenObject(ObjectType, sObjectId, Pt, Area, Released);
        if (ObjectType == windowId) {
            menubar = Area;

            if (Released) {
                userConfig->SetWindowState({menubar.left, menubar.top, isMinimized});
            }
        }
    }

    void InfoScreen::OnClickScreenObject(int ObjectType, const char *sObjectId, POINT Pt, RECT Area, int Button) {
        if (!canClick) {
            return;
        }

        canClick = false;
        if (ObjectType == closeId) {
            isOpen = false;
            RequestRefresh();
        } else if (ObjectType == minimizeId) {
            isMinimized = !isMinimized;
            userConfig->SetWindowState({menubar.left, menubar.top, isMinimized});
            RequestRefresh();
        } else if (ObjectType == statsToggleId) {
            showStats_ = !showStats_;
            RequestRefresh();
        } else if (ObjectType == authenticationButtonId) {
            switch (authService->GetAuthenticationState()) {
                case authentication::LOGIN:
                    authService->CancelAuthentication();
                    break;
                case authentication::REFRESH:
                    // NO OP
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
    }

    bool InfoScreen::OnCompileCommand(const char *sCommandLine) {
        if (_stricmp(sCommandLine, COMMAND_OPEN) == 0) {
            isOpen = true;
            return true;
        }

        if (_stricmp(sCommandLine, COMMAND_CLOSE) == 0) {
            isOpen = false;
            return true;
        }

        return false;
    }
}
