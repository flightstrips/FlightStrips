//
// Created by fsr19 on 10/01/2025.
//

#include "AppConfig.h"
#include <cctype>

namespace FlightStrips::configuration {
    std::string AppConfig::GetAuthority() {
        return std::string(ini["authentication"]["authority"] | "error");
    }

    std::string AppConfig::GetClientId() {
        return std::string(ini["authentication"]["clientId"] | "error");
    }

    std::string AppConfig::GetScopes() {
        return std::string(ini["authentication"]["scopes"] | "openid profile offline_access");
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

    CallsignAirportMap& AppConfig::GetCallsignAirportMap() {
        if (!callsignAirportMap.empty()) { return callsignAirportMap; }

        for (const auto [name, section] : ini) {
            auto nameStr = std::string(name);
            auto prefixesStr = std::string(section["callsignPrefixes"] | "");
            const auto airportsLength = std::strlen("airports.");
            if (_strnicmp(nameStr.c_str(), "airports.", airportsLength) == 0 && !prefixesStr.empty()) {
                auto airport = nameStr.substr(airportsLength);
                touppercase(airport);
                touppercase(prefixesStr);
                auto prefixes = split(prefixesStr, ' ');
                if (!prefixes.empty()) {
                    callsignAirportMap.emplace(airport, prefixes);
                }
            }
        }

        if (callsignAirportMap.empty()) {
            std::vector<std::string> callsigns = {"EKCH"};
            callsignAirportMap.emplace("EKCH", callsigns);
        }

        return callsignAirportMap;
    }
} // configuration
// FlightStrips