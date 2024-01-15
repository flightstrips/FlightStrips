#pragma once

#include "handlers/FlightPlanEventHandler.h"
#include "Server.h"
#include "runway/ActiveRunway.h"

namespace FlightStrips::stands {
    class StandService;
}

namespace FlightStrips::network {

    class NetworkService : public handlers::FlightPlanEventHandler {

    public:
        NetworkService(const std::shared_ptr<Server> &server, const std::shared_ptr<stands::StandService> &standService);

        void FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

        void ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) override;

        void FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

        void SquawkUpdateEvent(std::string callsign, int squawk) override;

        void SendActiveRunways(std::vector<runway::ActiveRunway> &runways) const;

    private:
        std::shared_ptr<Server> m_server;
        std::shared_ptr<stands::StandService> m_standService;

    };
}