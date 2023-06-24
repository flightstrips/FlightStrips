#pragma once

#include <functional>

#ifndef COPYRIGHTS
#define PLUGIN_NAME "FlightStrips"
#define PLUGIN_AUTHOR "Frederik Rosenberg"
#define PLUGIN_COPYRIGHT "GPLv3 License, Copyright (c) 2023 Frederik Rosenberg"
#define GITHUB_LINK "https://github.com/frederikrosenberg/FlightStrips"
#endif // !COPYRIGHTS

#include "handlers/FlightPlanEventHandlers.h"

namespace FlightStrips {
    class FlightStripsPlugin : public EuroScopePlugIn::CPlugIn {
    public:
        explicit FlightStripsPlugin(
                const std::shared_ptr<handlers::FlightPlanEventHandlers>& mFlightPlanEventHandlerCollection);

        ~FlightStripsPlugin() override;

        void Information(const std::string &message);

        void OnFlightPlanDisconnect (EuroScopePlugIn::CFlightPlan FlightPlan ) override;

        void OnFlightPlanControllerAssignedDataUpdate (EuroScopePlugIn::CFlightPlan FlightPlan, int DataType ) override;

        void OnFlightPlanFlightPlanDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan) override;

        void OnFlightPlanFlightStripPushed(EuroScopePlugIn::CFlightPlan FlightPlan,
                                           const char *sSenderController,
                                           const char *sTargetController) override;



        void OnTimer(int Counter) override;

    private:
        const std::shared_ptr<handlers::FlightPlanEventHandlers> m_flightPlanEventHandlerCollection;
    };
}
