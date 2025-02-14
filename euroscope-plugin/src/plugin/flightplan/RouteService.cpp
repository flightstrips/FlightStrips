#include "RouteService.h"

namespace FlightStrips::flightplan {
    std::vector<std::string> RouteService::GetBaseSids(const std::vector<Sid> &allSids) {
        auto baseSids = std::vector<std::string>(allSids.size());
        std::ranges::transform(allSids, baseSids.begin(), [](const Sid &x) { return GetBaseSid(x.name); });
        std::ranges::sort(baseSids);
        const auto iter = std::ranges::unique(baseSids).begin();
        baseSids.erase(iter, baseSids.end());
        return baseSids;
    }

    void RouteService::SetSid(std::string &route, const std::string &sid, const std::string &airport) const {
        if (airport.empty()) return;
        const auto allSids = m_plugin->GetSids(airport);
        if (allSids.empty()) {
            return;
        }
        auto upperSid = std::string(sid);
        ToUpperCase(upperSid);
        const auto baseSids = GetBaseSids(allSids);

        ltrim(route);
        ToUpperCase(route);
        EraseStartAirport(route, airport);

        const auto base = GetBaseSid(upperSid);
        const auto nextToken = GetNextToken(route);

        if (!nextToken.has_value()) {
            route = upperSid;
            return;
        }

        const auto nextBase = GetBaseSid(nextToken.value().token);
        if (std::ranges::find(std::as_const(baseSids), nextBase) != baseSids.end()) {
            EraseToken(route, nextToken.value());
        } else {
            route.insert(0, " ");
        }
        route.insert(0, upperSid);
    }

    void RouteService::SetDepartureRunway(std::string &route, const std::string &departureRunway,
                                          const std::string &airport) const {
        if (airport.empty()) return;
        const auto allSids = m_plugin->GetSids(airport);
        if (allSids.empty()) {
            return;
        }
        const auto baseSids = GetBaseSids(allSids);
        auto runwayToken = airport + "/" + departureRunway;
        ToUpperCase(runwayToken);
        ltrim(route);
        ToUpperCase(route);

        if (route.starts_with(runwayToken)) {
            return;
        }

        EraseStartAirport(route, airport);

        const auto nextToken = GetNextToken(route);
        if (!nextToken.has_value()) {
            // Weird case where route only included the airport
            route = runwayToken;
            return;
        }
        const auto nextBase = GetBaseSid(nextToken.value().token);
        if (std::ranges::find(std::as_const(baseSids), nextBase) != baseSids.end()) {
            route.erase(0, nextToken.value().length + 1);
            const auto baseSid = GetNextToken(route);
            if (!nextToken.has_value() || baseSid.value().token != nextBase) {
                route.insert(0, nextBase + ' ');
            }
        }

        route.insert(0, " ");
        route.insert(0, runwayToken);
    }

    std::string RouteService::GetBaseSid(const std::string &sid) {
        std::string result;
        for (int i = 0; i < sid.length(); i++) {
            if (isdigit(sid[i])) break;
            result.push_back(sid[i]);
        }

        return result;
    }

    bool RouteService::EraseStartAirport(std::string &route, const std::string &airport) {
        if (route.starts_with(airport)) {
            route.erase(0, route.find(' ') + 1);
            return true;
        }
        return false;
    }

    void RouteService::EraseToken(std::string &route, const Token &token) {
        route.erase(0, token.length);
    }

    std::optional<RouteService::Token> RouteService::GetNextToken(std::string &route) {
        const auto space = route.find(' ');
        if (space == std::string::npos) {
            return {};
        }

        const auto token = route.substr(0, space);
        return Token(token, space);
    }
}
