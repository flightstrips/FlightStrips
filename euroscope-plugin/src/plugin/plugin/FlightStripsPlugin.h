#pragma once

#ifndef COPYRIGHTS
#define PLUGIN_NAME "FlightStrips"
#define PLUGIN_VERSION "0.0.1"
#define PLUGIN_AUTHOR "Frederik Rosenberg"
#define PLUGIN_COPYRIGHT "GPLv3 License, Copyright (c) 2023 Frederik Rosenberg"
#define GITHUB_LINK "https://github.com/frederikrosenberg/FlightStrips"
#endif // !COPYRIGHTS

#include "authentication/AuthenticationService.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "handlers/RadarTargetEventHandlers.h"
#include "handlers/ControllerEventHandlers.h"
#include "handlers/TimedEventHandlers.h"
#include "handlers/AirportRunwaysChangedEventHandlers.h"
#include "runway/ActiveRunway.h"

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
                const std::shared_ptr<handlers::TimedEventHandlers> &mTimedEventHandlers,
                const std::shared_ptr<handlers::AirportRunwaysChangedEventHandlers> &mAirportRunwaysChangedEventHandlers,
                const std::shared_ptr<authentication::AuthenticationService> &mAuthenticationService,
                const std::shared_ptr<configuration::UserConfig> &mUserConfig);

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

        void SetClearenceFlag(const std::string &callsign, bool cleared);

        void UpdateViaScratchPad(const char* callsign, const char* message) const;
        std::vector<runway::ActiveRunway> GetActiveRunways(const char* airport) const;

        EuroScopePlugIn::CRadarScreen* OnRadarScreenCreated ( const char * sDisplayName, bool NeedRadarContent, bool GeoReferenced, bool CanBeSaved, bool CanBeCreated ) override;

        static bool ControllerIsMe(EuroScopePlugIn::CController controller, EuroScopePlugIn::CController me);

        static bool IsRelevant(EuroScopePlugIn::CFlightPlan flightPlan);
    private:
        const std::shared_ptr<handlers::FlightPlanEventHandlers> m_flightPlanEventHandlerCollection;
        const std::shared_ptr<handlers::RadarTargetEventHandlers> m_radarTargetEventHandlers;
        const std::shared_ptr<handlers::ControllerEventHandlers> m_controllerEventHandlerCollection;
        const std::shared_ptr<handlers::TimedEventHandlers> m_timedEventHandlers;
        const std::shared_ptr<handlers::AirportRunwaysChangedEventHandlers> m_airportRunwayChangedEventHandlers;
        const std::shared_ptr<authentication::AuthenticationService> m_authenticationService;
        const std::shared_ptr<configuration::UserConfig> m_userConfig;

    };
}
