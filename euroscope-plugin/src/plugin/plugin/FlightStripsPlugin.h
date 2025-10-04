#pragma once

#ifndef COPYRIGHTS
#define PLUGIN_NAME "FlightStrips"
#include "Version.h"
#define PLUGIN_AUTHOR "Frederik Rosenberg"
#define PLUGIN_COPYRIGHT "GPLv3 License, Copyright (c) 2025 Frederik Rosenberg"
#define GITHUB_LINK "https://github.com/flightstrips/FlightStrips"
#include "FlightStripsPluginInterface.h"
#include "bootstrap/Container.h"
#endif // !COPYRIGHTS

#include "authentication/AuthenticationService.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "handlers/RadarTargetEventHandlers.h"
#include "handlers/ControllerEventHandlers.h"
#include "handlers/TimedEventHandlers.h"
#include "handlers/AirportRunwaysChangedEventHandlers.h"

// TODO move
#define CLEARED "CLEA"
#define NOT_CLEARED "NOTC"

namespace FlightStrips {
    enum ConnectionType {
        CONNECTION_TYPE_NO               = 0,
        CONNECTION_TYPE_DIRECT           = 1,
        CONNECTION_TYPE_VIA_PROXY        = 2,
        CONNECTION_TYPE_SIMULATOR_SERVER = 3,
        CONNECTION_TYPE_PLAYBACK         = 4,
        CONNECTION_TYPE_SIMULATOR_CLIENT = 5,
        CONNECTION_TYPE_SWEATBOX         = 6
    };

    struct ConnectionState {
        int range;
        ConnectionType connection_type;
        std::string primary_frequency;
        std::string callsign;
        std::string relevant_airport;
    };

    class FlightStripsPlugin final : public EuroScopePlugIn::CPlugIn, public FlightStripsPluginInterface {
    public:
        FlightStripsPlugin(
                const std::shared_ptr<handlers::FlightPlanEventHandlers> &mFlightPlanEventHandlerCollection,
                const std::shared_ptr<handlers::RadarTargetEventHandlers> &mRadarTargetEventHandlers,
                const std::shared_ptr<handlers::ControllerEventHandlers> &mControllerEventHandlers,
                const std::shared_ptr<handlers::TimedEventHandlers> &mTimedEventHandlers,
                const std::shared_ptr<handlers::AirportRunwaysChangedEventHandlers> &mAirportRunwaysChangedEventHandlers,
                const std::weak_ptr<Container> &mContainer,
                const std::shared_ptr<configuration::AppConfig> &mAppConfig);

        ~FlightStripsPlugin() override;

        bool IsValidAirports(EuroScopePlugIn::CFlightPlan flightPlan) const;

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

        void SetClearenceFlag(const std::string &callsign, bool cleared) const;

        void SetArrivalStand(const std::string &callsign, std::string stand) const;

        void UpdateViaScratchPad(const char* callsign, const char* message) const;

        EuroScopePlugIn::CRadarScreen* OnRadarScreenCreated ( const char * sDisplayName, bool NeedRadarContent, bool GeoReferenced, bool CanBeSaved, bool CanBeCreated ) override;

        static bool ControllerIsMe(EuroScopePlugIn::CController controller, EuroScopePlugIn::CController me);

        [[nodiscard]] inline bool IsRelevant(EuroScopePlugIn::CFlightPlan flightPlan) const;

        ConnectionState& GetConnectionState();

        std::vector<Sid> GetSids(const std::string& airport) override;

        void AddNeedsSquawk(const std::string &callsign);
        std::optional<std::string> GetNeedsSquawk();

    private:
        const std::shared_ptr<handlers::FlightPlanEventHandlers> m_flightPlanEventHandlerCollection;
        const std::shared_ptr<handlers::RadarTargetEventHandlers> m_radarTargetEventHandlers;
        const std::shared_ptr<handlers::ControllerEventHandlers> m_controllerEventHandlerCollection;
        const std::shared_ptr<handlers::TimedEventHandlers> m_timedEventHandlers;
        const std::shared_ptr<handlers::AirportRunwaysChangedEventHandlers> m_airportRunwayChangedEventHandlers;
        const std::shared_ptr<configuration::AppConfig> m_appConfig;
        const std::weak_ptr<Container> m_container;

        ConnectionState m_connectionState = {};
        std::queue<std::string> m_needsSquawk = {};
    };
}
