//
// Created by fsr19 on 10/01/2025.
//

#pragma once
#include "Config.h"

typedef std::map<std::string, std::vector<std::string>> CallsignAirportMap;

namespace FlightStrips::configuration {

class AppConfig : public Config {
public:
    explicit AppConfig(const std::string &path)
        : Config(path) {
    }

    [[nodiscard]] std::string GetAuthority();
    [[nodiscard]] std::string GetClientId();
    [[nodiscard]] std::string GetScopes();
    [[nodiscard]] int GetRedirectPort();
    [[nodiscard]] std::string GetBaseUrl();
    [[nodiscard]] std::string GetLogLevel();
    [[nodiscard]] CallsignAirportMap& GetCallsignAirportMap();
private:
    CallsignAirportMap callsignAirportMap = {};

    static void touppercase(std::string &s) {
        for (char &c : s) { c = static_cast<char>(std::toupper(c)); }
    }

    static std::vector<std::string> split (const std::string &s, const char delim) {
        std::vector<std::string> result;
        std::stringstream ss (s);
        std::string item;

        while (getline (ss, item, delim)) {
            result.push_back (item);
        }

        return result;
    }
};

} // configuration
// FlightStrips

