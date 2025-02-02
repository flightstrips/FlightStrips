
#include "StandService.h"

#include <utility>

namespace FlightStrips::stands {
    StandService::StandService(std::vector<Stand> stands) : stands(std::move(stands)) {

    }

    Stand *StandService::GetStand(EuroScopePlugIn::CPosition position) {
        double min = 1000;
        Stand *stand = nullptr;

        for (auto &item: this->stands) {
            double distance = position.DistanceTo(item.GetPosition()) * NM_TO_METERS;
            if (distance < item.GetRadius() && distance < min) {
                stand = &item;
                min = distance;
            }
        }

        return stand;
    }

    Stand *StandService::GetStand(const std::string& standString, const std::string& airport) {
        if (standString.empty()) {
            return nullptr;
        }

        std::string::size_type first = standString.find_first_of("s/");
        std::string::size_type last = standString.find_last_of("s/");

        if (first == last || first == std::string::npos || last == std::string::npos) {
            return nullptr;
        }

        auto stand = standString.substr(first + 2, last - first - 3);

        auto matches = [airport, stand](Stand &item) {
            return std::strcmp(item.GetAirport().c_str(), airport.c_str()) == 0 &&
                   std::strcmp(item.GetName().c_str(), stand.c_str()) == 0;
        };

        auto iter = std::ranges::find_if(this->stands, matches);

        if (iter == this->stands.end()) {
            return nullptr;
        }

        return &*iter;
    }

    Stand *StandService::GetStandFromFlightPlan(EuroScopePlugIn::CFlightPlan flightPlan) {
        const auto trackPosition = flightPlan.GetFPTrackPosition();
        if (trackPosition.IsValid()) {
            const auto stand = this->GetStand(trackPosition.GetPosition());
            if (stand != nullptr) {
                return stand;
            }

        }

        const auto stand = this->GetStand(flightPlan.GetControllerAssignedData().GetFlightStripAnnotation(6), "EKCH");

        return stand;
    }

}

