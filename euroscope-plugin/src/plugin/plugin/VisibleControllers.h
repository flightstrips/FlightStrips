#pragma once
#include <algorithm>
#include <format>
#include "IFlightStripsPlugin.h"

namespace FlightStrips {
    // Read SDK handles only during this call on the SDK thread. The deployment
    // policy treats the enumerated list as complete, including non-plugin peers.
    template<typename Controller, typename First, typename Next>
    std::optional<std::vector<VisibleController>> ReadVisibleControllers(const Controller& me, First first, Next next) {
        if (!me.IsValid() || std::string(me.GetCallsign()).empty()) return std::nullopt;
        std::vector<VisibleController> controllers;
        const auto append = [&](const Controller& controller) {
            const std::string callsign = controller.GetCallsign();
            if (callsign.empty() || callsign.ends_with("_ATIS")) return;
            controllers.push_back({callsign, std::format("{:.3f}", controller.GetPrimaryFrequency()),
                controller.GetFacility() == 0 || callsign.ends_with("_OBS")});
        };
        // ControllerMyself is a separate SDK selection and may be absent from
        // the enumeration. Include it once, then the actual visible peers.
        append(me);
        for (auto controller = first(); controller.IsValid(); controller = next(controller)) {
            if (std::string(controller.GetCallsign()) != me.GetCallsign()) append(controller);
        }
        std::sort(controllers.begin(), controllers.end(), [](const auto& a, const auto& b) { return a.callsign < b.callsign; });
        return controllers;
    }
}
