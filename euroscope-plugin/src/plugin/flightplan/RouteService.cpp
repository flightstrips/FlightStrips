#include "RouteService.h"

namespace FlightStrips::flightplan {
    void RouteService::SetSid(std::string &route, const std::string &sid, const std::string &airport) const {
        if (airport.empty()) return;
        const auto allSids = m_plugin->GetSids(airport);
        if (allSids.empty()) {
            return;
        }

        auto baseSids = std::vector<std::string>(allSids.size());
        std::ranges::transform(allSids, baseSids.begin(), [](const Sid &x) { return GetBaseSid(x.name); });
        std::ranges::sort(baseSids);
        const auto iter = std::ranges::unique(baseSids).begin();
        baseSids.erase(iter, baseSids.end());

        ltrim(route);
        if (route.starts_with(airport)) {
            route.erase(0, route.find(' ') + 1);
        }

        const auto base = GetBaseSid(sid);
        const auto nextSpace = route.find(' ');
        if (nextSpace == std::string::npos) {
            // No next space
            route = sid;
            return;
        }

        const auto next = route.substr(0, nextSpace);
        const auto nextBase = GetBaseSid(next);

        if (std::ranges::find(std::as_const(baseSids), nextBase) != baseSids.end()) {
            route.erase(0, nextSpace);
        } else {
            route.insert(0, " ");
        }
        route.insert(0, sid);
    }

    std::string RouteService::GetBaseSid(const std::string &sid) {
        std::string result;
        for (int i = 0; i < sid.length(); i++) {
            if (isdigit(sid[i])) break;
            result.push_back(sid[i]);
        }

        return result;
    }
}
