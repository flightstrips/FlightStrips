//
// Created by fsr19 on 11/01/2025.
//

#include "InfoScreen.h"

#include "Colors.h"

namespace FlightStrips::graphics {
    InfoScreen::InfoScreen(
        const std::shared_ptr<authentication::AuthenticationService> &authenticationService,
        const std::shared_ptr<configuration::UserConfig> &config) : authService(
                                                                        authenticationService), userConfig(config) {
        const auto state = userConfig->GetWindowState();
        menubar = { state.x, state.y, state.x + width, state.y + height };
        isMinimized = state.minimized;
    }

    void InfoScreen::OnRefresh(HDC hDC, int Phase) {
        if (Phase != EuroScopePlugIn::REFRESH_PHASE_AFTER_LISTS) {
            return;
        }

        if (!isOpen) {
            return;
        }


        if (hdcHandle != hDC) {
            hdcHandle = hDC;
            graphics.SetHandle(hdcHandle);
        }

        AddScreenObject(windowId, "", menubar, true, nullptr);

        const RECT windowRect = {menubar.left, menubar.top, menubar.right, menubar.bottom + 75};
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
            return;
        }
        graphics.DrawRect(colors.backgroundPen.get(), windowRect);

        const RECT rectText = {menubar.left, menubar.bottom + 5, menubar.right, menubar.bottom + 35};

        std::string btnText;
        if (authService->IsRunningAuthentication()) {
            graphics.DrawString("Logging in...", rectText, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
            btnText = "Cancel";
        } else if (authService->IsAuthenticated()) {
            graphics.DrawString(std::format("Logged in as:\n{}", authService->GetName()), rectText,
                                colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
            btnText = "Logout";
        } else {
            graphics.DrawString("Logged out.", rectText, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
            btnText = "Login";
        }

        const RECT btnRect = {rectText.left + 10, rectText.bottom + 10, rectText.right - 70, rectText.bottom + 30};
        graphics.FillRect(colors.headerBrush.get(), btnRect);
        graphics.DrawString(btnText, btnRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentCenter);
        AddScreenObject(authenticationButtonId, "", btnRect, false, nullptr);
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
            if (authService->IsRunningAuthentication()) {
                authService->CancelAuthentication();
            } else if (authService->IsAuthenticated()) {
                authService->Logout();
            } else if (!authService->IsAuthenticated()) {
                authService->StartAuthentication();
            }
            RequestRefresh();
        }
    }
}
