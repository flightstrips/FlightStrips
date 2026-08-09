#pragma once

#include <string>

namespace FlightStrips {
    constexpr int EKYT_APP_AUTO_ASSUME_MAX_ALTITUDE_FEET = 12500;

    [[nodiscard]] bool ShouldAutoAssumeAndDropEkytApproachHandoff(
        const std::string& controllerCallsign,
        const std::string& primaryFrequency,
        bool observer,
        bool transferToMeInitiated,
        bool radarPositionValid,
        int pressureAltitudeFeet);

    [[nodiscard]] bool ShouldAutoDropEkytApproachAircraft(
        const std::string& controllerCallsign,
        const std::string& primaryFrequency,
        bool observer,
        bool flightPlanAssumed,
        bool trackingControllerIsMe);
}
