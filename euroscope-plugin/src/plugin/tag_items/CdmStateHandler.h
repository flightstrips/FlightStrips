#pragma once

#include "TagItemHandler.h"
#include "flightplan/FlightPlanService.h"

namespace FlightStrips::TagItems {
    class CdmStateHandler final : public TagItemHandler {
    public:
        enum class Field {
            Eobt,
            Phase,
            Tobt,
            ReqTobt,
            Tsat,
            TsatTobtDiff,
            Ttg,
            Ttot,
            Ctot,
            FlowMessage,
            Status,
            TobtConfirmedBy,
            Asrt,
            ReadyStartup,
            Tsac,
            Asat
        };

        struct Presentation {
            std::string value{};
            COLORREF color = 0;
            bool hasValue = false;
        };

        CdmStateHandler(
            std::shared_ptr<flightplan::FlightPlanService> flightPlanService,
            Field field
        ) : m_flightPlanService(std::move(flightPlanService)), m_field(field) {
        }

        static auto ResolvePresentation(const flightplan::CdmState& cdm, Field field, const std::string& fallbackEobt = {}) -> Presentation;

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
        Field m_field;
    };
}
