#include "AMANGainLossHandler.h"

#include <cctype>
#include <cstdio>
#include <format>

namespace FlightStrips::TagItems {
    namespace {
        constexpr int TagColorRGBDefinedValue = 1;
        constexpr COLORREF ActiveTagColor = RGB(0, 192, 0);

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
                                             std::function<bool()> connected)
        : store_(std::move(store)), connected_(std::move(connected)) {}

    void AMANGainLossHandler::Handle(EuroScopePlugIn::CFlightPlan flightPlan, EuroScopePlugIn::CRadarTarget,
                                     int, int, char sItemString[16], int* pColorCode,
                                     COLORREF* pRGB, double*) {
        const auto callsign = flightPlan.IsValid() ? std::string(flightPlan.GetCallsign()) : std::string{};
        const auto presentation = Resolve(connected_ && connected_(), store_ ? store_->Snapshot() : nullptr, callsign);
        std::snprintf(sItemString, 16, "%s", presentation.text.c_str());
        if (pColorCode != nullptr) *pColorCode = TagColorRGBDefinedValue;
        if (pRGB != nullptr) *pRGB = presentation.color;
    }

    auto AMANGainLossHandler::Format(const long long seconds) -> std::string {
        const auto magnitude = seconds < 0
            ? static_cast<unsigned long long>(-(seconds + 1)) + 1
            : static_cast<unsigned long long>(seconds);
        if (magnitude < 30) return "=00";
        const auto minutes = (magnitude + 30) / 60;
        const char prefix = seconds < 0 ? 'L' : 'G';
        if (minutes > 99) return std::format("{}99+", prefix);
        return std::format("{}{:02}", prefix, minutes);
    }

    auto AMANGainLossHandler::Resolve(const bool connected,
                                      const std::shared_ptr<const aman::GainLossSnapshot>& snapshot,
                                      const std::string& callsign) -> AMANGainLossPresentation {
        if (!connected || !snapshot || !snapshot->authoritative) return {"----", ActiveTagColor};
        const auto index = snapshot->flightIdByCallsign.find(Normalize(callsign));
        if (index == snapshot->flightIdByCallsign.end()) return {"----", ActiveTagColor};
        const auto value = snapshot->byFlightId.find(index->second);
        if (value == snapshot->byFlightId.end() || value->second.dataStatus != "fresh" || !value->second.seconds) {
            return {"----", ActiveTagColor};
        }
        return {Format(*value->second.seconds), ActiveTagColor};
    }
}
