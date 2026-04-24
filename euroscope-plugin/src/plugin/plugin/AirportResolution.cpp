#include "AirportResolution.h"

#include <cmath>
#include <cctype>
#include <limits>

namespace FlightStrips {
    namespace {
        auto ToUpperCopy(std::string value) -> std::string {
            for (char& c : value) {
                c = static_cast<char>(std::toupper(static_cast<unsigned char>(c)));
            }
            return value;
        }

        auto MakePosition(const double latitude, const double longitude) -> EuroScopePlugIn::CPosition {
            EuroScopePlugIn::CPosition position;
            position.m_Latitude = latitude;
            position.m_Longitude = longitude;
            return position;
        }
    }

    std::string ResolveAirportFromCallsign(const std::string& callsign, const CallsignAirportMap& airportMap) {
        const auto normalizedCallsign = ToUpperCopy(callsign);
        for (const auto& [airport, prefixes] : airportMap) {
            for (const auto& prefix : prefixes) {
                const auto normalizedPrefix = ToUpperCopy(prefix);
                if (normalizedCallsign.starts_with(normalizedPrefix)) {
                    return airport;
                }
            }
        }

        return {};
    }

    std::optional<std::string> ResolveAirportFromPosition(const EuroScopePlugIn::CPosition& controllerPosition,
                                                          const std::vector<configuration::AirportFallbackPoint>& airports) {
        if (airports.empty()) {
            return std::nullopt;
        }

        auto bestDistance = std::numeric_limits<double>::infinity();
        std::optional<std::string> bestAirport;
        for (const auto& airport : airports) {
            const auto airportPosition = MakePosition(airport.latitude, airport.longitude);
            const auto distance = controllerPosition.DistanceTo(airportPosition);
            if (distance <= AIRPORT_FALLBACK_RADIUS_NM && distance < bestDistance) {
                bestDistance = distance;
                bestAirport = airport.airport;
            }
        }

        return bestAirport;
    }

    bool HasSameAirportFallbackProbe(const std::optional<AirportFallbackProbe>& previous,
                                     const std::string& callsign,
                                     const EuroScopePlugIn::CPosition& controllerPosition) {
        if (!previous.has_value()) {
            return false;
        }

        return previous->callsign == callsign
            && std::abs(previous->latitude - controllerPosition.m_Latitude) <= AIRPORT_FALLBACK_POSITION_EPSILON
            && std::abs(previous->longitude - controllerPosition.m_Longitude) <= AIRPORT_FALLBACK_POSITION_EPSILON;
    }
}
