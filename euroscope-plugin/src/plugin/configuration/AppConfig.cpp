//
// Created by fsr19 on 10/01/2025.
//

#include "AppConfig.h"

namespace FlightStrips::configuration {
    std::string AppConfig::GetAuthority() {
        return std::string(ini["authentication"]["authority"] | "error");
    }

    std::string AppConfig::GetAudience() {
        return std::string(ini["authentication"]["audience"] | "error");
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

    bool AppConfig::GetApiEnabled() {
        return ini["api"]["enabled"] | false;
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

    DeIceConfig& AppConfig::GetDeIceConfig() {
        if (!deIceConfig.order.empty()) { return deIceConfig; }

        for (const auto [name, section] : ini) {
            // TODO support more airports
            if (!name.starts_with("deice_designator")) continue;

            if (auto order = std::string(section["order"] | ""); !order.empty()) {
                deIceConfig.order = split(order, ' ');
            }

            if (auto fallback = std::string(section["default"] | ""); !fallback.empty()) {
                deIceConfig.fallback = fallback;
            }

            iterate_deice_type(section, "ac_type_rule_", deIceConfig.ac_types);
            iterate_deice_type(section, "airline_rule_", deIceConfig.airlines);
            iterate_deice_type(section, "stand_rule_", deIceConfig.stands);
        }

        return deIceConfig;
    }

    int AppConfig::GetPositionUpdateIntervalSeconds() {
        return ini["api"]["position_update_interval_seconds"] | 10;
    }

    std::string AppConfig::GetStandsFile() {
        return std::string(ini["files"]["stands"] | "GRpluginStands.txt");
    }
} // configuration
// FlightStrips