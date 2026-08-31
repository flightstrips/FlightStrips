#pragma once

#include <string>
#include <string_view>

namespace FlightStrips::flightplan {
    constexpr int TOPSKY_HOLD_ANNOTATION = 6;

    // TopSky stores a holding clearance in two places. The scratch pad carries a
    // transient "/HOLD/<point>/" pulse that EuroScope broadcasts to controllers in
    // range and TopSky then wipes; annotation 6 carries the durable "h/<point>/h".
    // Only the annotation is state - reading the scratch pad gives a hold that
    // vanishes a millisecond after it appears.
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

    // An absent time means "nothing new" rather than "cleared", the pulse being
    // transient, so callers keep the previous value.
    std::string ParseTopSkyHoldEat(std::string_view scratchPad);
}
