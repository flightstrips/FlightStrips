#pragma once

#include <optional>
#include <string>

#include "Colors.h"
#include "Graphics.h"
#include "authentication/IAuthenticationService.h"
#include "euroscope/EuroScopePlugIn.h"
#include "plugin/IFlightStripsPlugin.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips::graphics {
    struct InfoPanelData {
        authentication::AuthenticationState authenticationState{authentication::NONE};
        std::string authenticatedName;
        ConnectionState connectionState{};
        bool sessionSelectable{false};
        std::string effectiveSession;
        bool connected{false};
        bool backingOff{false};
        bool pending{false};
        std::optional<int> delaySeconds;
        websocket::Stats stats{};
        bool showStats{true};
    };

    struct AuthenticationButtonLayout {
        RECT authenticationButtonRect{};
        std::optional<RECT> openAppButtonRect;
    };

    [[nodiscard]] int CalculateInfoPanelContentHeight(const InfoPanelData& data);
    [[nodiscard]] AuthenticationButtonLayout CalculateAuthenticationButtonLayout(const InfoPanelData& data, int left, int right, int y);
    [[nodiscard]] std::string GetInfoPanelRoleLabel(websocket::ClientState role);

    void DrawInfoPanel(EuroScopePlugIn::CRadarScreen& screen,
                       Graphics& graphics,
                       const Colors& colors,
                       const RECT& menubar,
                       const InfoPanelData& data,
                       bool isMinimized);
}
