#pragma once

#include "handlers/FlightPlanEventHandler.h"
#include "runway/ActiveRunway.h"
#include "handlers/ControllerEventHandler.h"
#include "handlers/TimedEventHandler.h"
#include "handlers/RadarTargetEventHandler.h"
#include "FlightStripsClient.h"

#include <unordered_map>
#include <plugin/FlightStripsPlugin.h>

namespace FlightStrips::stands {
    class StandService;
}

namespace FlightStrips::network {

    class NetworkService
            : public handlers::FlightPlanEventHandler,
              public handlers::ControllerEventHandler,
              public handlers::TimedEventHandler,
              public handlers::RadarTargetEventHandler {

    public:
        explicit NetworkService(const std::shared_ptr<FlightStripsPlugin>& plugin, const std::shared_ptr<grpc::Channel> &channel);
        ~NetworkService() override;

        void FlightPlanEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

        void ControllerFlightPlanDataEvent(EuroScopePlugIn::CFlightPlan flightPlan, int dataType) override;

        void FlightPlanDisconnectEvent(EuroScopePlugIn::CFlightPlan flightPlan) override;

        void SquawkUpdateEvent(std::string callsign, std::string squawk) override;

        void ControllerPositionUpdateEvent(EuroScopePlugIn::CController controller) override;

        void ControllerDisconnectEvent(EuroScopePlugIn::CController controller) override;

        void OnTimer(int time) override;

        void RadarTargetPositionEvent(EuroScopePlugIn::CRadarTarget radarTarget) override;


    private:
        static Capabilities GetCapabilities(const EuroScopePlugIn::CFlightPlan& flightPlan);
        void OnNetworkMessage(const ServerStreamMessage& message);

        std::shared_ptr<FlightStripsPlugin> plugin;
        enum State {
            NOT_SENT = 0,
            ONLINE = 1,
            OFFLINE = 2,
            OUT_OF_RANGE = 3
        };

        const int RANGE = 50;
        std::string airport = "EKCH";
        const int DELAY_IN_SECONDS = 10;
        const double DEFAULT_FREQUENCY = 199.980;

        bool isMaster = false;
        bool initialized = false;
        double frequency = 0;
        std::string position;


        int connectionStatus = EuroScopePlugIn::CONNECTION_TYPE_NO;
        int onlineTime = 0;

        //std::unordered_map<std::string, State> strips;
        std::unique_ptr<Reader> reader;
        FlightStripsClient client;

        [[nodiscard]] bool ShouldSend() const;
        [[nodiscard]] static bool Online(int connection);

        static CommunicationType GetCommunicationType(const EuroScopePlugIn::CFlightPlan &flightPlan);
        static CommunicationType GetCommunicationType(char type);
        static GroundState GetGroundState(const EuroScopePlugIn::CFlightPlan &flightPlan);
        static WeightCategory GetAircraftWtc(const EuroScopePlugIn::CFlightPlan &flightPlan);

    };
}