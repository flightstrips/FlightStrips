#include "ClearanceStatusHandler.h"

namespace FlightStrips::TagItems {
    namespace {
        constexpr int TAG_COLOR_RGB_DEFINED_VALUE = 1;
        constexpr COLORREF TAG_GREEN  = RGB(110, 153, 110);
        constexpr COLORREF TAG_YELLOW = RGB(212, 214, 7);
        constexpr COLORREF TAG_RED    = RGB(190, 0, 0);
        constexpr auto REQUEST_WITH_REMARKS = "REQ*";
    }

    ClearanceStatusHandler::Presentation ClearanceStatusHandler::ResolvePresentation(
        const flightplan::FlightPlan& plan,
        const bool esCleared
    ) {
        if (plan.IsPdcConfirmed()) {
            return {.hasValue = true, .value = "DONE", .color = TAG_GREEN};
        }

        if (esCleared) {
            return {.hasValue = true, .value = "OK", .color = TAG_GREEN};
        }

        if (plan.pdc_state.empty()) {
            return {};
        }

        if (plan.pdc_state == "REQUESTED") {
            return {.hasValue = true, .value = plan.pdc_request_remarks.empty() ? "REQ" : REQUEST_WITH_REMARKS, .color = TAG_GREEN};
        }
        if (plan.pdc_state == "REQUESTED_WITH_FAULTS") {
            return {.hasValue = true, .value = plan.pdc_request_remarks.empty() ? "REQ" : REQUEST_WITH_REMARKS, .color = TAG_YELLOW};
        }
        if (plan.IsPdcCleared()) {
            return {.hasValue = true, .value = "SENT", .color = TAG_GREEN};
        }
        if (plan.pdc_state == "NO_RESPONSE" || plan.pdc_state == "FAILED") {
            return {.hasValue = true, .value = "FAIL", .color = TAG_RED};
        }
        if (plan.pdc_state == "REVERT_TO_VOICE") {
            return {.hasValue = true, .value = "R/T", .color = TAG_GREEN};
        }

        return {};
    }

    void ClearanceStatusHandler::Handle(
        EuroScopePlugIn::CFlightPlan FlightPlan,
        EuroScopePlugIn::CRadarTarget,
        int,
        int,
        char sItemString[16],
        int* pColorCode,
        COLORREF* pRGB,
        double*
    ) {
        if (m_flightPlanService == nullptr || !FlightPlan.IsValid()) return;
        const flightplan::FlightPlan emptyPlan{};
        const auto* trackedPlan = m_flightPlanService->GetFlightPlan(std::string(FlightPlan.GetCallsign()));
        const auto presentation = ResolvePresentation(
            trackedPlan == nullptr ? emptyPlan : *trackedPlan,
            FlightPlan.GetClearenceFlag()
        );

        if (!presentation.hasValue) return;

        std::snprintf(sItemString, 16, "%s", presentation.value.c_str());
        if (pColorCode != nullptr) *pColorCode = TAG_COLOR_RGB_DEFINED_VALUE;
        if (pRGB != nullptr) *pRGB = presentation.color;
    }
}
