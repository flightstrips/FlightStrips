//
// Created by fsr19 on 10/01/2025.
//

#include "AppConfig.h"

namespace FlightStrips::configuration {
    std::string AppConfig::GetAuthority() {
        return std::string(ini["authentication"]["authority"] | "error");
    }

    std::string AppConfig::GetClientId() {
        return std::string(ini["authentication"]["clientId"] | "error");
    }

    int AppConfig::GetRedirectPort() {
        return ini["authentication"]["redirectPort"] | 27015;
    }

    std::string AppConfig::GetBaseUrl() {
        return std::string(ini["api"]["baseurl"] | "error");
    }

    std::string AppConfig::GetLogLevel() {
        return std::string(ini["logging"]["level"] | "INFO");
    }
} // configuration
// FlightStrips