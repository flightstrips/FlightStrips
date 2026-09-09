#pragma once

#include <functional>
#include <memory>
#include <string>

#include "TagItemHandler.h"
#include "aman/AMANGainLossStore.h"

namespace FlightStrips::TagItems {
    struct AMANGainLossPresentation {
        std::string text = "----";
        COLORREF color = RGB(0, 192, 0);
    };

    class AMANGainLossHandler final : public TagItemHandler {
    public:
        AMANGainLossHandler(std::shared_ptr<aman::AMANGainLossStore> store, std::function<bool()> connected);

        void Handle(EuroScopePlugIn::CFlightPlan FlightPlan, EuroScopePlugIn::CRadarTarget RadarTarget,
                    int ItemCode, int TagData, char sItemString[16], int* pColorCode,
                    COLORREF* pRGB, double* pFontSize) override;

        [[nodiscard]] static std::string Format(long long seconds);
        [[nodiscard]] static AMANGainLossPresentation Resolve(
            bool connected, const std::shared_ptr<const aman::GainLossSnapshot>& snapshot,
            const std::string& callsign);

    private:
        std::shared_ptr<aman::AMANGainLossStore> store_;
        std::function<bool()> connected_;
    };
}
