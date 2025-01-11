//
// Created by fsr19 on 10/01/2025.
//

#pragma once
#include "Config.h"


namespace FlightStrips::configuration {

struct Token {
    std::string accessToken;
    std::string refreshToken;
    std::string idToken;
    time_t expiry;
};

struct WindowState {
    int x;
    int y;
    bool minimized;
};

class UserConfig : public Config {
public:
    explicit UserConfig(const std::string &path)
        : Config(path) {
    }

    [[nodiscard]] Token GetToken();
    void SetToken(const Token& token);

    [[nodiscard]] WindowState GetWindowState();
    void SetWindowState(const WindowState& state);

private:
    const char* TokenSection = "token";
    const char* WindowSection = "window";


};

} // configuration
// FlightStrips

