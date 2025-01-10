//
// Created by fsr19 on 10/01/2025.
//

#pragma once
#include "Config.h"


namespace FlightStrips::configuration {

class AppConfig : public Config {
public:
    explicit AppConfig(const std::string &path)
        : Config(path) {
    }

    [[nodiscard]] std::string GetAuthority();
    [[nodiscard]] std::string GetClientId();
    [[nodiscard]] int GetRedirectPort();
    [[nodiscard]] std::string GetBaseUrl();
    [[nodiscard]] std::string GetLogLevel();
};

} // configuration
// FlightStrips

