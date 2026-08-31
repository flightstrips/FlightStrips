#include "TopSkyHold.h"

#include <algorithm>

namespace FlightStrips::flightplan {
    namespace {
        constexpr std::string_view ENROUTE_OPEN = "h/";
        constexpr std::string_view ENROUTE_CLOSE = "/h";
        constexpr std::string_view TSA_OPEN = "t/";
        constexpr std::string_view TSA_CLOSE = "/t";
        constexpr std::string_view EAT_TOKEN = "/HOLD_EAT/";
        constexpr std::string_view EAT_CLOSE = "/";

        constexpr std::size_t MAXIMUM_POINT_LENGTH = 12;

        bool LooksLikeHoldingPoint(const std::string_view value) {
            if (value.empty() || value.size() > MAXIMUM_POINT_LENGTH) return false;
            return std::all_of(value.begin(), value.end(), [](const unsigned char character) {
                return std::isalnum(character) != 0 || character == '-' || character == '_';
            });
        }

        std::string Between(
            const std::string_view haystack,
            const std::string_view open,
            const std::string_view close,
            const bool validate
        ) {
            const auto start = haystack.find(open);
            if (start == std::string_view::npos) return {};

            const auto valueStart = start + open.size();
            const auto valueEnd = haystack.find(close, valueStart);
            if (valueEnd == std::string_view::npos) return {};

            const auto value = haystack.substr(valueStart, valueEnd - valueStart);
            if (validate && !LooksLikeHoldingPoint(value)) return {};
            return std::string(value);
        }
    }

    TopSkyHold ParseTopSkyHoldAnnotation(const std::string_view annotation) {
        TopSkyHold hold{};
        if (annotation.empty()) return hold;

        if (auto area = Between(annotation, TSA_OPEN, TSA_CLOSE, true); !area.empty()) {
            hold.active = true;
            hold.tsa = true;
            hold.point = std::move(area);
            return hold;
        }

        if (auto point = Between(annotation, ENROUTE_OPEN, ENROUTE_CLOSE, true); !point.empty()) {
            hold.active = true;
            hold.point = std::move(point);
        }

        return hold;
    }

    std::string ParseTopSkyHoldEat(const std::string_view scratchPad) {
        if (scratchPad.empty()) return {};
        return Between(scratchPad, EAT_TOKEN, EAT_CLOSE, false);
    }
}
