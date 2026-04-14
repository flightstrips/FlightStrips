#include "InfoPanel.h"

#include "InfoScreenObjectIds.h"

namespace FlightStrips::graphics {
    namespace {
        std::string FormatCount(const int n) {
            if (n < 1000) return std::to_string(n);
            if (n < 1000000) return std::format("{:.1f}K", n / 1000.0);
            return std::format("{:.1f}M", n / 1000000.0);
        }
    }

    int CalculateInfoPanelContentHeight(const InfoPanelData& data) {
        int contentHeight = 83 + (data.sessionSelectable ? 28 : 16) + 25 + 5;
        if (data.showStats) {
            if (data.connected) {
                contentHeight += 39;
            }
            contentHeight += 16;
        }

        return contentHeight;
    }

    AuthenticationButtonLayout CalculateAuthenticationButtonLayout(const InfoPanelData& data, const int left, const int right, const int y) {
        if (data.authenticationState != authentication::AUTHENTICATED) {
            return {
                .authenticationButtonRect = {left + 5, y + 38, right - 70, y + 54},
                .openAppButtonRect = std::nullopt,
            };
        }

        return {
            .authenticationButtonRect = {left + 5, y + 38, left + 73, y + 54},
            .openAppButtonRect = RECT{left + 77, y + 38, right - 5, y + 54},
        };
    }

    void DrawInfoPanel(EuroScopePlugIn::CRadarScreen& screen,
                       Graphics& graphics,
                       const Colors& colors,
                       const RECT& menubar,
                       const InfoPanelData& data,
                       const bool isMinimized) {
        screen.AddScreenObject(InfoScreenObjectIds::Window, "", menubar, true, nullptr);

        const RECT closeButton = {menubar.right - 12, menubar.top + 3, menubar.right - 3, menubar.bottom - 3};
        screen.AddScreenObject(InfoScreenObjectIds::CloseButton, "", closeButton, false, nullptr);
        const RECT minimizeButton = {menubar.right - 30, menubar.top + 3, menubar.right - 18, menubar.bottom - 3};
        screen.AddScreenObject(InfoScreenObjectIds::MinimizeButton, "", minimizeButton, false, nullptr);

        if (!isMinimized) {
            const RECT windowRect = {menubar.left, menubar.top, menubar.right, menubar.bottom + CalculateInfoPanelContentHeight(data)};
            graphics.FillRect(colors.backgroundBrush.get(), windowRect);
            graphics.DrawRect(colors.backgroundPen.get(), windowRect);
        }

        const Gdiplus::Brush* dotBrush = data.connected ? colors.greenBrush.get()
                                       : data.backingOff ? colors.redBrush.get()
                                       : data.pending    ? colors.orangeBrush.get()
                                                         : colors.redBrush.get();

        graphics.FillRect(colors.headerBrush.get(), menubar);
        graphics.DrawString("FlightStrips", menubar, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
        graphics.DrawXButton(colors.buttonPen.get(), closeButton);
        graphics.DrawLineButton(colors.buttonPen.get(), minimizeButton);
        const RECT headerDot = {menubar.right - 44, menubar.top + 4, menubar.right - 36, menubar.top + 12};
        graphics.FillEllipse(dotBrush, headerDot);

        if (isMinimized) {
            return;
        }

        const int left = menubar.left + 5;
        const int right = menubar.right - 5;
        int y = menubar.bottom;

        const RECT authTextRect = {left, y + 4, right, y + 34};
        std::string buttonText;
        switch (data.authenticationState) {
            case authentication::LOGIN:
                graphics.DrawString("Logging in...", authTextRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                buttonText = "Cancel";
                break;
            case authentication::REFRESH:
                graphics.DrawString("Refreshing token...", authTextRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                buttonText = "No action";
                break;
            case authentication::AUTHENTICATED:
                graphics.DrawString(std::format("Logged in as:\n{}", data.authenticatedName),
                                    authTextRect,
                                    colors.whiteBrush.get(),
                                    Gdiplus::StringAlignmentNear);
                buttonText = "Logout";
                break;
            case authentication::NONE:
            default:
                graphics.DrawString("Logged out.", authTextRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
                buttonText = "Login";
                break;
        }

        const auto authButtonLayout = CalculateAuthenticationButtonLayout(data, left, right, y);
        graphics.FillRect(colors.headerBrush.get(), authButtonLayout.authenticationButtonRect);
        graphics.DrawString(buttonText,
                            authButtonLayout.authenticationButtonRect,
                            colors.whiteBrush.get(),
                            Gdiplus::StringAlignmentCenter);
        screen.AddScreenObject(InfoScreenObjectIds::AuthenticationButton, "", authButtonLayout.authenticationButtonRect, false, nullptr);
        if (authButtonLayout.openAppButtonRect.has_value()) {
            graphics.FillRect(colors.headerBrush.get(), authButtonLayout.openAppButtonRect.value());
            graphics.DrawString("Open App",
                                authButtonLayout.openAppButtonRect.value(),
                                colors.whiteBrush.get(),
                                Gdiplus::StringAlignmentCenter);
            screen.AddScreenObject(InfoScreenObjectIds::OpenAppButton, "", authButtonLayout.openAppButtonRect.value(), false, nullptr);
        }

        y += 58;
        graphics.DrawHLine(colors.separatorPen.get(), menubar.left + 3, y + 4, menubar.right - 3);
        y += 9;

        if (data.sessionSelectable) {
            const RECT sessionLabelRect = {left, y, right, y + 12};
            graphics.DrawString("Session mode", sessionLabelRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
            y += 12;

            const RECT liveButtonRect = {left, y, left + 70, y + 16};
            const RECT sweatboxButtonRect = {left + 74, y, right, y + 16};
            const bool liveSelected = !data.connectionState.prefer_sweatbox;
            const bool sweatboxSelected = data.connectionState.prefer_sweatbox;

            const auto drawModeButton = [&](const RECT& rect, const char* text, const bool selected) {
                graphics.FillRect(selected ? colors.greenBrush.get() : colors.backgroundBrush.get(), rect);
                graphics.DrawRect(colors.buttonPen.get(), rect);
                graphics.DrawString(text,
                                    rect,
                                    selected ? colors.backgroundBrush.get() : colors.whiteBrush.get(),
                                    Gdiplus::StringAlignmentCenter);
            };

            drawModeButton(liveButtonRect, "LIVE", liveSelected);
            drawModeButton(sweatboxButtonRect, "SWEATBOX", sweatboxSelected);
            screen.AddScreenObject(InfoScreenObjectIds::SessionLiveButton, "", liveButtonRect, false, nullptr);
            screen.AddScreenObject(InfoScreenObjectIds::SessionSweatboxButton, "", sweatboxButtonRect, false, nullptr);
            y += 16;
        } else {
            const RECT sessionRect = {left, y, right, y + 16};
            graphics.DrawString(std::format("Session  {}", data.effectiveSession),
                                sessionRect,
                                colors.whiteBrush.get(),
                                Gdiplus::StringAlignmentNear);
            y += 16;
        }

        const std::string statusText = data.connected ? "Connected"
                                     : data.backingOff ? (data.delaySeconds.has_value() ? std::format("Retry in {}s", data.delaySeconds.value()) : "Retrying...")
                                     : data.pending    ? (data.delaySeconds.has_value() ? std::format("Syncing  ({}s)", data.delaySeconds.value()) : "Connecting...")
                                                       : "Disconnected";

        const RECT dotRect = {left, y + 4, left + 8, y + 12};
        graphics.FillEllipse(dotBrush, dotRect);
        const RECT statusLabelRect = {left + 13, y, right, y + 16};
        graphics.DrawString(statusText, statusLabelRect, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
        y += 19;

        graphics.DrawHLine(colors.separatorPen.get(), menubar.left + 3, y + 3, menubar.right - 3);
        y += 8;

        const RECT statsToggleRect = {left, y, right, y + 14};
        screen.AddScreenObject(InfoScreenObjectIds::StatsToggle, "", statsToggleRect, false, nullptr);
        graphics.DrawString(data.showStats ? "Stats  [-]" : "Stats  [+]",
                            statsToggleRect,
                            colors.whiteBrush.get(),
                            Gdiplus::StringAlignmentNear);
        y += 14;

        if (!data.showStats) {
            return;
        }

        if (data.connected) {
            const std::string connectionType = GetEffectiveSessionShortName(data.connectionState);
            const std::string role = data.stats.role == websocket::STATE_MASTER ? "MASTER"
                                     : data.stats.role == websocket::STATE_SLAVE ? "SLAVE"
                                                                                  : "SYNC";

            const RECT infoRow1 = {left + 2, y, right, y + 13};
            graphics.DrawString(data.connectionState.callsign, infoRow1, colors.whiteBrush.get(), Gdiplus::StringAlignmentNear);
            y += 13;

            const RECT infoRow2 = {left + 2, y, right, y + 13};
            graphics.DrawString(std::format("{}  {}", data.connectionState.primary_frequency, data.connectionState.relevant_airport),
                                infoRow2,
                                colors.whiteBrush.get(),
                                Gdiplus::StringAlignmentNear);
            y += 13;

            const RECT infoRow3 = {left + 2, y, right, y + 13};
            graphics.DrawString(std::format("{}  {}", role, connectionType),
                                infoRow3,
                                colors.whiteBrush.get(),
                                Gdiplus::StringAlignmentNear);
            y += 13;
        }

        y += 3;
        const RECT statsRect = {left + 2, y, right, y + 13};
        graphics.DrawString(std::format("TX {}   RX {}   Q {}",
                                        FormatCount(data.stats.tx),
                                        FormatCount(data.stats.rx),
                                        FormatCount(data.stats.queued)),
                            statsRect,
                            colors.whiteBrush.get(),
                            Gdiplus::StringAlignmentNear);
    }
}
