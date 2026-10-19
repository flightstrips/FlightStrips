//
// Created by fsr19 on 10/01/2025.
//

#include "UserConfig.h"

namespace FlightStrips {
namespace configuration {
    bool UserConfig::GetVacsEnabled() { return ini["vacs"]["enabled"] | false; }
    bool UserConfig::GetVacsShowInMinimized() { return ini["vacs"]["show_in_minimized"] | false; }
    std::string UserConfig::GetVacsHost() { return ini["vacs"]["host"] | "localhost"; }
    void UserConfig::SetVacsEnabled(bool enabled) { ini["vacs"]["enabled"] = enabled; save(); }
    void UserConfig::SetVacsShowInMinimized(bool show) { ini["vacs"]["show_in_minimized"] = show; save(); }
    Token UserConfig::GetToken() {
        const auto section = ini[TokenSection];
        return {
            std::string(section["access_token"] | ""),
            std::string(section["refresh_token"] | ""),
            std::string(section["id_token"] | ""),
            section["expiry"] | 0
        };
    }

    void UserConfig::SetToken(const Token& token) {
        const auto section = ini[TokenSection];
        section["access_token"] = token.accessToken;
        section["refresh_token"] = token.refreshToken;
        section["id_token"] = token.idToken;
        section["expiry"] = token.expiry;
        save();
    }

    WindowState UserConfig::GetWindowState() {
        const auto section = ini[WindowSection];
        return WindowState {
            section["x"] | 400,
            section["y"] | 400,
            section["minimized"] | false
        };
    }

    void UserConfig::SetWindowState(const WindowState& state) {
        const auto section = ini[WindowSection];
        section["x"] = state.x;
        section["y"] = state.y;
        section["minimized"] = state.minimized;
        save();
    }

    bool UserConfig::GetPreferSweatboxSession() {
        const auto section = ini[SessionSection];
        return section["prefer_sweatbox"] | false;
    }

    void UserConfig::SetPreferSweatboxSession(const bool preferSweatbox) {
        const auto section = ini[SessionSection];
        section["prefer_sweatbox"] = preferSweatbox;
        save();
    }
} // configuration
} // FlightStrips
