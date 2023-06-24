#pragma once

#include "handlers/FlightPlanEventHandler.h"
#include "Server.h"

namespace FlightStrips::network {
    class NetworkService : public handlers::FlightPlanEventHandler {

    public:
        explicit NetworkService(const std::shared_ptr<Server> &server);

        void FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

        void ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) override;

        void FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

    private:
        std::shared_ptr<Server> m_server;

        static bool IsRelevant(EuroScopePlugIn::CFlightPlan flightPlan);

    };

}