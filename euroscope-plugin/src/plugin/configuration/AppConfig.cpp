//
// Created by fsr19 on 10/01/2025.
//

#include "AppConfig.h"

#include <algorithm>
#include <sstream>

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

    std::vector<int> AppConfig::GetRedirectPorts() {
        const auto configuredPorts = std::string(ini["authentication"]["redirectPorts"] | "");
        std::istringstream ports(configuredPorts);
        std::vector<int> result;
        int port;

        const auto addPort = [&result](const int candidate) {
            if (candidate > 0 && candidate <= 65535 && std::find(result.begin(), result.end(), candidate) == result.end()) {
                result.push_back(candidate);
            }
        };

        while (ports >> port) {
            addPort(port);
        }

        if (result.empty()) {
            addPort(ini["authentication"]["redirectPort"] | 27015);
            addPort(32015);
            addPort(37015);
            addPort(42015);
            addPort(47015);
        }

        return result;
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

    std::vector<AirportFallbackPoint>& AppConfig::GetAirportFallbackPoints() {
        if (!airportFallbackPoints.empty()) { return airportFallbackPoints; }

        for (const auto [name, section] : ini) {
            auto nameStr = std::string(name);
            const auto airportsLength = std::strlen("airports.");
            if (_strnicmp(nameStr.c_str(), "airports.", airportsLength) != 0) {
                continue;
            }

            auto airport = nameStr.substr(airportsLength);
            touppercase(airport);

            const auto latitude = parse_double(section, "latitude");
            const auto longitude = parse_double(section, "longitude");
            if (!latitude.has_value() || !longitude.has_value()) {
                continue;
            }

            airportFallbackPoints.push_back(AirportFallbackPoint{
                airport,
                *latitude,
                *longitude,
            });
        }

        return airportFallbackPoints;
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

    std::string AppConfig::GetAirlinesFile() {
        return std::string(ini["files"]["airlines"] | "../../ICAO/ICAO_Airlines.txt");
    }

    bool AppConfig::GetDisconnectOnOutOfRange() {
        return ini["api"]["disconnect_on_out_of_range"] | false;
    }
} // configuration
// FlightStrips
