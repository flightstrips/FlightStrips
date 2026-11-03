#pragma once

#include <optional>
#include <string>
#include <string_view>

namespace FlightStrips {
    struct RequestOnFrequency {
        std::string aircraftCallsign;
        std::string controllerCallsign;
    };

    std::optional<RequestOnFrequency> ParseRequestOnFrequency(std::string_view scratchPad);

    bool ShouldReleaseGroundTag(
        const std::string& ownerCallsign,
        int ownerFacility,
        const std::string& requesterCallsign,
        int requesterFacility,
        bool observer,
        bool trackingControllerIsMe);
}
