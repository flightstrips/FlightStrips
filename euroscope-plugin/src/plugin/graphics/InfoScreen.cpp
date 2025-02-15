//
// Created by fsr19 on 11/01/2025.
//

#include "InfoScreen.h"

#include "Colors.h"
#include "plugin/FlightStripsPlugin.h"

namespace FlightStrips::graphics {
    InfoScreen::InfoScreen(
        const std::shared_ptr<authentication::AuthenticationService> &authenticationService,
        const std::shared_ptr<configuration::UserConfig> &config,
        const std::weak_ptr<websocket::WebSocketService> &webSocketService,
        FlightStripsPlugin *plugin) : authService(
                                          authenticationService), userConfig(config), webSocketService(webSocketService), m_plugin(plugin) {
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

        AddScreenObject(windowId, "", menubar, true, nullptr);

        const RECT windowRect = {menubar.left, menubar.top, menubar.right, menubar.bottom + 110};
        const RECT closeBth = {menubar.right - 12, menubar.top + 3, menubar.right - 3, menubar.bottom - 3};
        AddScreenObject(closeId, "", closeBth, false, nullptr);
        const RECT minimizeBtn = {menubar.right - 30, menubar.top + 3, menubar.right - 18, menubar.bottom - 3};
        AddScreenObject(minimizeId, "", minimizeBtn, false, nullptr);

        if (!isMinimized) {
            graphics.FillRect(colors.backgroundBrush.get(), windowRect);
        }
        graphics.FillRect(colors.headerBrush.get(), menubar);
        graphics.DrawString("FlightStrips", menubar, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
        graphics.DrawXButton(colors.buttonPen.get(), closeBth);
        graphics.DrawLineButton(colors.buttonPen.get(), minimizeBtn);

        if (isMinimized) {
            canClick = true;
            return;
        }
        graphics.DrawRect(colors.backgroundPen.get(), windowRect);

        const RECT rectText = {menubar.left, menubar.bottom + 5, menubar.right, menubar.bottom + 35};

        std::string btnText;

        switch (authService->GetAuthenticationState()) {
            case authentication::LOGIN:
                graphics.DrawString("Logging in...", rectText, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                btnText = "Cancel";
                break;
            case authentication::REFRESH:
                graphics.DrawString("Refreshing token...", rectText, colors.whiteBrush.get(),
                                    Gdiplus::StringAlignmentNear);
                btnText = "No action";
                break;
            case authentication::AUTHENTICATED:
                graphics.DrawString(std::format("Logged in as:\n{}", authService->GetName()), rectText,
                                    colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                btnText = "Logout";
                break;
            case authentication::NONE:
            default:
                graphics.DrawString("Logged out.", rectText, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                btnText = "Login";
                break;
        }

        const RECT btnRect = {rectText.left + 10, rectText.bottom + 10, rectText.right - 70, rectText.bottom + 30};
        graphics.FillRect(colors.headerBrush.get(), btnRect);
        graphics.DrawString(btnText, btnRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentCenter);
        AddScreenObject(authenticationButtonId, "", btnRect, false, nullptr);

        if (const auto service = webSocketService.lock()) {
            const RECT statusRect = {rectText.left, btnRect.bottom + 10, rectText.right, btnRect.bottom + 45};
            const auto [tx, rx] = service->GetStats();
            const auto statusText = std::format("{}\nTX: {}\nRX: {}", service->IsConnected() ? "Connected" : "Disconnected", tx, rx);
            graphics.DrawString(statusText, statusRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
        }

        canClick = true;
    }

    void InfoScreen::OnAsrContentToBeClosed() {
        if (hdcHandle) {
            hdcHandle = nullptr;
        }
        delete this;
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
