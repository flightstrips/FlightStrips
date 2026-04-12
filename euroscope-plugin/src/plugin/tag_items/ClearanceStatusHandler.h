#pragma once

#include "TagItemHandler.h"
#include "flightplan/FlightPlanService.h"

namespace FlightStrips::TagItems {
    class ClearanceStatusHandler final : public TagItemHandler {
    public:
        struct Presentation final {
            bool hasValue{false};
            std::string value{};
            COLORREF color{};
        };

        explicit ClearanceStatusHandler(std::shared_ptr<flightplan::FlightPlanService> flightPlanService)
            : m_flightPlanService(std::move(flightPlanService)) {}

        static Presentation ResolvePresentation(const flightplan::FlightPlan& plan, bool esCleared);

        void Handle(
            EuroScopePlugIn::CFlightPlan FlightPlan,
            EuroScopePlugIn::CRadarTarget RadarTarget,
            int ItemCode,
            int TagData,
            char sItemString[16],
            int* pColorCode,
            COLORREF* pRGB,
            double* pFontSize
        ) override;

    private:
        std::shared_ptr<flightplan::FlightPlanService> m_flightPlanService;
    };
}
