#include "SharedFieldHandler.h"
#include <cstdio>

namespace FlightStrips::TagItems {
    std::string SharedFieldHandler::Resolve(const flightplan::FlightPlan* plan, const Field field) {
        if (plan == nullptr) return {};
        if (field == Field::ScratchPad) return plan->fs_scratch_pad;
        if (plan->published_hold_eat) return *plan->published_hold_eat;
        if (plan->backend_hold_eat_replay) return plan->backend_hold_eat_replay->eat;
        return plan->hold.empty() ? std::string{} : plan->hold_eat;
    }

    void SharedFieldHandler::Handle(EuroScopePlugIn::CFlightPlan flightPlan, EuroScopePlugIn::CRadarTarget,
                                   int, int, char text[16], int*, COLORREF*, double*) {
        text[0] = '\0';
        if (!flightPlan.IsValid() || !service_) return;
        const auto value = Resolve(service_->GetFlightPlan(flightPlan.GetCallsign()), field_);
        std::snprintf(text, 16, "%s", value.c_str());
    }
}
