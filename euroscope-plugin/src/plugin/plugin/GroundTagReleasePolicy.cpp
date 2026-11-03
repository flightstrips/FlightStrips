#include "GroundTagReleasePolicy.h"

#include <algorithm>
#include <cctype>

namespace FlightStrips {
    namespace {
        bool IsCallsign(const std::string_view callsign) {
            return !callsign.empty() && std::all_of(callsign.begin(), callsign.end(), [](const unsigned char character) {
                return (character >= 'A' && character <= 'Z')
                    || (character >= 'a' && character <= 'z')
                    || (character >= '0' && character <= '9')
                    || character == '_';
            });
        }

        bool HasSuffix(std::string callsign, const std::string_view suffix) {
            std::transform(callsign.begin(), callsign.end(), callsign.begin(), [](const unsigned char character) {
                return static_cast<char>(std::toupper(character));
            });
            return callsign.size() > suffix.size() && callsign.ends_with(suffix);
        }
    }

    std::optional<RequestOnFrequency> ParseRequestOnFrequency(const std::string_view scratchPad) {
        constexpr std::string_view prefix = "/ROF/";
        if (!scratchPad.starts_with(prefix)) return std::nullopt;

        const auto fields = scratchPad.substr(prefix.size());
        const auto separator = fields.find('/');
        if (separator == std::string_view::npos) return std::nullopt;

        const auto aircraft = fields.substr(0, separator);
        const auto controller = fields.substr(separator + 1);
        if (!IsCallsign(aircraft) || !IsCallsign(controller)) return std::nullopt;

        return RequestOnFrequency{std::string(aircraft), std::string(controller)};
    }

    bool ShouldReleaseGroundTag(
        const std::string& ownerCallsign,
        const int ownerFacility,
        const std::string& requesterCallsign,
        const int requesterFacility,
        const bool observer,
        const bool trackingControllerIsMe) {
        return !observer && trackingControllerIsMe
            && ownerFacility == 4 && HasSuffix(ownerCallsign, "_TWR")
            && ((requesterFacility == 5 && HasSuffix(requesterCallsign, "_APP"))
                || (requesterFacility == 6 && HasSuffix(requesterCallsign, "_CTR")));
    }
}
