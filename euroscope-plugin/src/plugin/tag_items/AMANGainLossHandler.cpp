#include "AMANGainLossHandler.h"

#include <cctype>
#include <cstdio>
#include <format>

namespace FlightStrips::TagItems {
    namespace {
        constexpr int TagColorRGBDefinedValue = 1;
        constexpr COLORREF ActiveTagColor = RGB(0, 192, 0);
        constexpr COLORREF GainOrOnTimeTagColor = RGB(150, 215, 150);
        constexpr COLORREF ShortLossTagColor = RGB(240, 225, 41);
        constexpr COLORREF LongLossTagColor = RGB(156, 0, 0);

        auto Magnitude(const long long seconds) -> unsigned long long {
            return seconds < 0
                ? static_cast<unsigned long long>(-(seconds + 1)) + 1
                : static_cast<unsigned long long>(seconds);
        }

        auto DisplayedMinutes(const long long seconds) -> unsigned long long {
            return (Magnitude(seconds) + 30) / 60;
        }

        auto GuidanceColor(const long long seconds) -> COLORREF {
            if (seconds >= 0 || Magnitude(seconds) < 30) return GainOrOnTimeTagColor;
            return DisplayedMinutes(seconds) >= 4 ? LongLossTagColor : ShortLossTagColor;
        }

        auto Normalize(std::string callsign) -> std::string {
            const auto first = callsign.find_first_not_of(" \t\r\n");
            if (first == std::string::npos) return "";
            callsign.erase(0, first);
            callsign.erase(callsign.find_last_not_of(" \t\r\n") + 1);
            for (auto& character : callsign) {
                character = static_cast<char>(std::toupper(static_cast<unsigned char>(character)));
            }
            return callsign;
        }
    }

    AMANGainLossHandler::AMANGainLossHandler(std::shared_ptr<aman::AMANGainLossStore> store,
                                             std::function<bool()> connected,
                                             std::function<std::string()> currentAirport)
        : store_(std::move(store)), connected_(std::move(connected)), currentAirport_(std::move(currentAirport)) {}

    void AMANGainLossHandler::Handle(EuroScopePlugIn::CFlightPlan flightPlan, EuroScopePlugIn::CRadarTarget,
                                     int, int, char sItemString[16], int* pColorCode,
                                     COLORREF* pRGB, double*) {
        const auto callsign = flightPlan.IsValid() ? std::string(flightPlan.GetCallsign()) : std::string{};
        const auto destination = flightPlan.IsValid()
            ? std::string(flightPlan.GetFlightPlanData().GetDestination())
            : std::string{};
        const auto currentAirport = currentAirport_ ? currentAirport_() : std::string{};
        const auto presentation = Resolve(
            connected_ && connected_(), store_ ? store_->Snapshot() : nullptr, callsign, destination, currentAirport);
        if (presentation.text.empty()) return;
        std::snprintf(sItemString, 16, "%s", presentation.text.c_str());
        if (pColorCode != nullptr) *pColorCode = TagColorRGBDefinedValue;
        if (pRGB != nullptr) *pRGB = presentation.color;
    }

    auto AMANGainLossHandler::Format(const long long seconds) -> std::string {
        const auto magnitude = Magnitude(seconds);
        if (magnitude < 30) return "=00";
        const auto minutes = DisplayedMinutes(seconds);
        const char prefix = seconds < 0 ? 'L' : 'G';
        if (minutes > 99) return std::format("{}99+", prefix);
        return std::format("{}{:02}", prefix, minutes);
    }

    auto AMANGainLossHandler::Resolve(const bool connected,
                                      const std::shared_ptr<const aman::GainLossSnapshot>& snapshot,
                                      const std::string& callsign, const std::string& destination,
                                      const std::string& currentAirport) -> AMANGainLossPresentation {
        const auto normalizedAirport = Normalize(currentAirport);
        if (normalizedAirport.empty() || Normalize(destination) != normalizedAirport) return {"", ActiveTagColor};
        if (!connected || !snapshot || !snapshot->authoritative) return {"----", ActiveTagColor};
        const auto index = snapshot->flightIdByCallsign.find(Normalize(callsign));
        if (index == snapshot->flightIdByCallsign.end()) return {"----", ActiveTagColor};
        const auto value = snapshot->byFlightId.find(index->second);
        if (value == snapshot->byFlightId.end() || value->second.dataStatus != "fresh" || !value->second.seconds) {
            return {"----", ActiveTagColor};
        }
        return {Format(*value->second.seconds), GuidanceColor(*value->second.seconds)};
    }
}
