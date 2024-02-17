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
#include "handlers/ControllerEventHandlers.h"

// TODO move
#define CLEARED "CLEA"
#define NOT_CLEARED "NOTC"

namespace FlightStrips {
    class FlightStripsPlugin : public EuroScopePlugIn::CPlugIn {
    public:
        FlightStripsPlugin(
                const std::shared_ptr<handlers::FlightPlanEventHandlers> &mFlightPlanEventHandlerCollection,
                const std::shared_ptr<handlers::RadarTargetEventHandlers> &mRadarTargetEventHandlers,
                const std::shared_ptr<handlers::ControllerEventHandlers> &mControllerEventHandlers,
                const std::shared_ptr<network::NetworkService> &mNetworkService);

        ~FlightStripsPlugin() override;

        void Information(const std::string &message);
        void Error(const std::string &message);

        void OnFlightPlanDisconnect (EuroScopePlugIn::CFlightPlan FlightPlan ) override;

        void OnFlightPlanControllerAssignedDataUpdate (EuroScopePlugIn::CFlightPlan FlightPlan, int DataType ) override;

        void OnFlightPlanFlightPlanDataUpdate(EuroScopePlugIn::CFlightPlan FlightPlan) override;

        void OnFlightPlanFlightStripPushed(EuroScopePlugIn::CFlightPlan FlightPlan,
                                           const char *sSenderController,
                                           const char *sTargetController) override;

        void OnRadarTargetPositionUpdate (EuroScopePlugIn::CRadarTarget RadarTarget) override;

        void OnAirportRunwayActivityChanged() override;

        void OnControllerPositionUpdate (EuroScopePlugIn::CController Controller ) override;
        void OnControllerDisconnect (EuroScopePlugIn::CController Controller ) override;

        void OnTimer(int Counter) override;

        void SetClearenceFlag(std::string callsign, bool cleared);

        void UpdateViaScratchPad(const char* callsign, const char* message) const;

    private:
        const std::shared_ptr<handlers::FlightPlanEventHandlers> m_flightPlanEventHandlerCollection;
        const std::shared_ptr<handlers::RadarTargetEventHandlers> m_radarTargetEventHandlers;
        const std::shared_ptr<handlers::ControllerEventHandlers> m_controllerEventHandlerCollection;
        const std::shared_ptr<network::NetworkService> m_networkService;

        int connectionType = 0;

        static bool IsRelevant(EuroScopePlugIn::CFlightPlan flightPlan);
        static bool IsRelevant(EuroScopePlugIn::CController controller);
        std::unique_ptr<std::thread> readerThread;
    };
}
