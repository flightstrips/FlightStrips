//
// Created by fsr19 on 10/01/2025.
//

#pragma once
#include <unordered_map>

#include "Config.h"

typedef std::map<std::string, std::vector<std::string>> CallsignAirportMap;

namespace FlightStrips::configuration {
    struct DeIceConfig {
        std::vector<std::string> order;
        std::unordered_map<std::string, std::string> ac_types;
        std::unordered_map<std::string, std::string> airlines;
        std::unordered_map<std::string, std::string> stands;
        std::string fallback;
    };

class AppConfig : public Config {
public:
    explicit AppConfig(const std::string &path)
        : Config(path) {
    }

    [[nodiscard]] std::string GetAuthority();
    [[nodiscard]] std::string GetAudience();
    [[nodiscard]] std::string GetClientId();
    [[nodiscard]] std::string GetScopes();
    [[nodiscard]] int GetRedirectPort();
    [[nodiscard]] std::string GetBaseUrl();
    [[nodiscard]] bool GetApiEnabled();
    [[nodiscard]] std::string GetLogLevel();
    [[nodiscard]] CallsignAirportMap& GetCallsignAirportMap();
    [[nodiscard]] DeIceConfig& GetDeIceConfig();
    [[nodiscard]] int GetPositionUpdateIntervalSeconds();
private:
    CallsignAirportMap callsignAirportMap = {};
    DeIceConfig deIceConfig;

    static void touppercase(std::string &s) {
        for (char &c : s) { c = static_cast<char>(std::toupper(c)); }
    }

    static void iterate_deice_type(const tortellini::ini::section &section, const std::string &prefix, std::unordered_map<std::string, std::string> &map) {
        for (auto i = 1; i <= 10; i++) {
            const auto key = prefix + std::to_string(i);
            const auto line = section[key] | "";
            if (line.empty()) break;
            parse_deice_type(line, map);
        }
    }

    static void parse_deice_type(const std::string &line, std::unordered_map<std::string, std::string> &map) {
        const auto index = line.find("->");
        if (index == std::string::npos) return;
        const auto keys_str = line.substr(0, index);
        auto result = line.substr(index + 2);
        std::erase_if(result, ::isspace);

        for (auto keys = split(keys_str, ' '); auto &key : keys) {
            std::erase_if(key, ::isspace);
            touppercase(key);
            map.try_emplace(key, result);
        }
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

