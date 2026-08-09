#include "ApproachHandoffPolicy.h"

#include <cstring>

namespace FlightStrips {
    namespace {
        bool IsEligibleEkytApproachPosition(
            const std::string& controllerCallsign,
            const std::string& primaryFrequency,
            const bool observer) {
            return !observer
                && controllerCallsign.size() >= 4
                && _strnicmp(controllerCallsign.c_str(), "EKYT", 4) == 0
                && (primaryFrequency == "123.980" || primaryFrequency == "120.705");
        }
    }

    bool ShouldAutoAssumeAndDropEkytApproachHandoff(
        const std::string& controllerCallsign,
        const std::string& primaryFrequency,
        const bool observer,
        const bool transferToMeInitiated,
        const bool radarPositionValid,
        const int pressureAltitudeFeet) {
        return IsEligibleEkytApproachPosition(controllerCallsign, primaryFrequency, observer)
            && transferToMeInitiated
            && radarPositionValid
            && pressureAltitudeFeet <= EKYT_APP_AUTO_ASSUME_MAX_ALTITUDE_FEET;
    }

    bool ShouldAutoDropEkytApproachAircraft(
        const std::string& controllerCallsign,
        const std::string& primaryFrequency,
        const bool observer,
        const bool flightPlanAssumed,
        const bool trackingControllerIsMe) {
        return IsEligibleEkytApproachPosition(controllerCallsign, primaryFrequency, observer)
            && flightPlanAssumed
            && trackingControllerIsMe;
    }
}
