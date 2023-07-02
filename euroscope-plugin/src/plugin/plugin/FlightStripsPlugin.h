#pragma once

#include <functional>

#ifndef COPYRIGHTS
#define PLUGIN_NAME "FlightStrips"
#define PLUGIN_AUTHOR "Frederik Rosenberg"
#define PLUGIN_COPYRIGHT "GPLv3 License, Copyright (c) 2023 Frederik Rosenberg"
#define GITHUB_LINK "https://github.com/frederikrosenberg/FlightStrips"
#endif // !COPYRIGHTS

#include "handlers/FlightPlanEventHandlers.h"
#include "handlers/RadarTargetEventHandlers.h"
#include "network/NetworkService.h"

namespace FlightStrips {
    class FlightStripsPlugin : public EuroScopePlugIn::CPlugIn {
    public:
        FlightStripsPlugin(
                const std::shared_ptr<handlers::FlightPlanEventHandlers> &mFlightPlanEventHandlerCollection,
                const std::shared_ptr<handlers::RadarTargetEventHandlers> &mRadarTargetEventHandlers,
                const std::shared_ptr<network::NetworkService> mNetworkService);

        ~FlightStripsPlugin() override;

        void Information(const std::string &message);

        void OnFlightPlanDisconnect (EuroScopePlugIn::CFlightPlan FlightPlan ) override;

        void OnFlightPlanControllerAssignedDataUpdate (EuroScopePlugIn::CFlightPlan FlightPlan, int DataType ) override;

        void OnFlightPlanFlightPlanDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan) override;

        void OnFlightPlanFlightStripPushed(EuroScopePlugIn::CFlightPlan FlightPlan,
                                           const char *sSenderController,
                                           const char *sTargetController) override;

        void OnRadarTargetPositionUpdate (EuroScopePlugIn::CRadarTarget RadarTarget) override;

        void OnAirportRunwayActivityChanged() override;



        void OnTimer(int Counter) override;

    private:
        const std::shared_ptr<handlers::FlightPlanEventHandlers> m_flightPlanEventHandlerCollection;
        const std::shared_ptr<handlers::RadarTargetEventHandlers> m_radarTargetEventHandlers;
        const std::shared_ptr<network::NetworkService> m_networkService;

        static bool IsRelevant(EuroScopePlugIn::CFlightPlan flightPlan);
    };
}
