#pragma once
#include "TagItemHandler.h"
#include "flightplan/FlightPlanService.h"

namespace FlightStrips::TagItems {
    // EuroScope invokes the same handler for tag items and flight-list columns.
    class SharedFieldHandler final : public TagItemHandler {
    public:
        enum class Field { Eat, ScratchPad };
        SharedFieldHandler(std::shared_ptr<flightplan::FlightPlanService> service, Field field)
            : service_(std::move(service)), field_(field) {}
        static std::string Resolve(const flightplan::FlightPlan* plan, Field field);
        void Handle(EuroScopePlugIn::CFlightPlan flightPlan, EuroScopePlugIn::CRadarTarget,
                    int, int, char text[16], int*, COLORREF*, double*) override;
    private:
        std::shared_ptr<flightplan::FlightPlanService> service_;
        Field field_;
    };
}
