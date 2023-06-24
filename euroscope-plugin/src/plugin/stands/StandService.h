//
// Created by fsr19 on 21/05/2023.
//

#pragma once

#include "Stand.h"
#define NM_TO_METERS 1852.00f

namespace FlightStrips::stands {
    class StandService {
    public:
        explicit StandService(std::vector<Stand> stands);
        Stand* GetStand(EuroScopePlugIn::CPosition position);
        Stand* GetStand(const std::string& stand, const std::string& airport);
        Stand* GetStandFromFlightPlan(EuroScopePlugIn::CFlightPlan flightPlan);

    private:
        // this may end up being a map but should make no change to the caller
        std::vector<Stand> stands;
    };
}
