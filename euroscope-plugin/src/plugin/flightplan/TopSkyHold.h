#pragma once

#include <string>
#include <string_view>

namespace FlightStrips::flightplan {
    constexpr int TOPSKY_HOLD_ANNOTATION = 6;

    enum class TopSkyHoldCommandType {
        None,
        Assign,
        Cancel,
        Eat,
    };

    struct TopSkyHoldCommand final {
        TopSkyHoldCommandType type{TopSkyHoldCommandType::None};
        std::string value{};
        std::string eat{};
    };

    // TopSky broadcasts holding commands through the scratch pad. Annotation 6
    // may contain a local durable copy, but it is not guaranteed to be present on
    // remote controllers and must therefore only be used for reconciliation.
    struct TopSkyHold final {
        bool active{false};
        bool tsa{false};
        std::string point{};

        [[nodiscard]] std::string TypeName() const {
            if (!active) return {};
            return tsa ? "tsa" : "enroute";
        }

        bool operator==(const TopSkyHold& other) const {
            return active == other.active && tsa == other.tsa && point == other.point;
        }

        bool operator!=(const TopSkyHold& other) const { return !(*this == other); }
    };

    // Annotation 6 is shared with GroundRadar's GRP/S/ stands, so the token is
    // located within the slot and its payload validated. Never write this slot.
    TopSkyHold ParseTopSkyHoldAnnotation(std::string_view annotation);

    // Parses the live TopSky scratch-pad protocol. HOLD assigns an en-route
    // hold, XHOLD cancels it (with or without a point), and HOLD_EAT changes its
    // expect-approach time. Trailing TopSky data after the point is ignored.
    TopSkyHoldCommand ParseTopSkyHoldCommand(std::string_view scratchPad);

    // Retained for callers that only need the EAT value.
    std::string ParseTopSkyHoldEat(std::string_view scratchPad);

    // Builds the transient TopSky command only when the backend update matches
    // the durable en-route hold currently present in EuroScope.
    std::string BuildTopSkyHoldEatCommand(
        const TopSkyHold& current,
        std::string_view expectedPoint,
        std::string_view expectedType,
        std::string_view hhmm);
}
