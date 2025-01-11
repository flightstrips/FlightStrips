//
// Created by fsr19 on 10/01/2025.
//

#include "UserConfig.h"

namespace FlightStrips {
namespace configuration {
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
} // configuration
} // FlightStrips