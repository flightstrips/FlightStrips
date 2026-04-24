#pragma once

#include <optional>
#include <string>
#include <vector>

#include "configuration/AppConfig.h"
#include "euroscope/EuroScopePlugIn.h"

namespace FlightStrips {
    constexpr double AIRPORT_FALLBACK_RADIUS_NM = 10.0;
    constexpr double AIRPORT_FALLBACK_POSITION_EPSILON = 0.0001;

    struct AirportFallbackProbe {
        std::string callsign;
        double latitude;
        double longitude;
    };

    [[nodiscard]] std::string ResolveAirportFromCallsign(const std::string& callsign,
                                                         const CallsignAirportMap& airportMap);
    [[nodiscard]] std::optional<std::string> ResolveAirportFromPosition(
        const EuroScopePlugIn::CPosition& controllerPosition,
        const std::vector<configuration::AirportFallbackPoint>& airports);
    [[nodiscard]] bool HasSameAirportFallbackProbe(const std::optional<AirportFallbackProbe>& previous,
                                                   const std::string& callsign,
                                                   const EuroScopePlugIn::CPosition& controllerPosition);
}
