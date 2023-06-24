//
// Created by fsr19 on 24/06/2023.
//

#include "NetworkService.h"
#include "euroscope/JsonConversion.h"

namespace FlightStrips::network {
    NetworkService::NetworkService(const std::shared_ptr<FlightStrips::network::Server> &server)
        : m_server(server) {
    }

    void NetworkService::FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
        if (!IsRelevant(flightPlan)) {
            return;
        }

        json j = flightPlan.GetFlightPlanData();
        j["callsign"] = flightPlan.GetCallsign();
        j["$type"] = "FlightPlanUpdated";
        auto message = j.dump();

        this->m_server->SendMessage(message);
    }

    void NetworkService::ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan,
                                                                              int dataType) {
    }

    void NetworkService::FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) {
    }

    bool NetworkService::IsRelevant(EuroScopePlugIn::CFlightPlan flightPlan) {
        return strcmp(flightPlan.GetFlightPlanData().GetDestination(), AIRPORT) == 0
            || strcmp(flightPlan.GetFlightPlanData().GetOrigin(), AIRPORT) == 0;
    }
}

