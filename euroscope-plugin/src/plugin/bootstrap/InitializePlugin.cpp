#include "InitializePlugin.h"

#include "Logger.h"
#include "authentication/AuthenticationService.h"
#include "configuration/AppConfig.h"
#include "plugin/FlightStripsPlugin.h"
#include "filesystem/FileSystem.h"
#include "stands/StandsBootstrapper.h"
#include "euroscope/EuroScopePlugIn.h"
#include "handlers/ControllerEventHandlers.h"
#include "handlers/TimedEventHandlers.h"
#include "handlers/AirportRunwaysChangedEventHandlers.h"
#include "configuration/ConfigurationBootstrapper.h"
#include "controller/ControllerService.h"
#include "flightplan/FlightPlanBootstrapper.h"
#include "flightplan/RouteService.h"
#include "handlers/ConnectionEventHandlers.h"
#include "messages/MessageService.h"
#include "runway/RunwayService.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips {
    auto InitializePlugin::GetPlugin() -> EuroScopePlugIn::CPlugIn * {
        return this->container->plugin.get();
    }

    void InitializePlugin::PostInit(HINSTANCE dllInstance) {
        this->container = std::make_shared<Container>();
        this->container->filesystem = std::make_unique<filesystem::FileSystem>(dllInstance);
        FilghtStrips::configuration::ConfigurationBootstrapper::Bootstrap(*this->container);
        Logger::LOG_PATH = this->container->filesystem->GetLocalFilePath("flightstrips.log").string();
        Logger::SetLevelFromString(this->container->appConfig->GetLogLevel());
        Logger::Debug("Logger initialized and loaded configuration!");

        this->container->connectionEventHandlers = std::make_shared<handlers::ConnectionEventHandlers>();
        this->container->controllerEventHandlers = std::make_shared<handlers::ControllerEventHandlers>();
        this->container->flightPlanEventHandlers = std::make_shared<handlers::FlightPlanEventHandlers>();
        this->container->radarTargetEventHandlers = std::make_shared<handlers::RadarTargetEventHandlers>();
        this->container->timedEventHandlers = std::make_shared<handlers::TimedEventHandlers>();
        this->container->messageHandlers = std::make_shared<handlers::MessageHandlers>();
        this->container->authenticationEventHandlers = std::make_shared<handlers::AuthenticationEventHandlers>();
        this->container->airportRunwaysChangedEventHandlers = std::make_shared<
            handlers::AirportRunwaysChangedEventHandlers>();
        stands::StandsBootstrapper::Bootstrap(*this->container);

        this->container->authenticationService = std::make_shared<authentication::AuthenticationService>(
            this->container->appConfig, this->container->userConfig, this->container->authenticationEventHandlers);
        this->container->timedEventHandlers->RegisterHandler(this->container->authenticationService);
        this->container->plugin = std::make_shared<FlightStripsPlugin>(this->container->flightPlanEventHandlers,
                                                                       this->container->radarTargetEventHandlers,
                                                                       this->container->controllerEventHandlers,
                                                                       this->container->timedEventHandlers,
                                                                       this->container->
                                                                       airportRunwaysChangedEventHandlers,
                                                                       this->container->authenticationService,
                                                                       this->container->userConfig,
                                                                       this->container->appConfig);
        this->container->webSocketService = std::make_shared<websocket::WebSocketService>(
            this->container->appConfig, this->container->authenticationService, this->container->plugin,
            this->container->connectionEventHandlers, this->container->messageHandlers);
        flightplan::FlightPlanBootstrapper::Bootstrap(*this->container);
        this->container->controllerService = std::make_shared<controller::ControllerService>(
            this->container->webSocketService);
        this->container->controllerEventHandlers->RegisterHandler(this->container->controllerService);
        this->container->runwayService = std::make_shared<runway::RunwayService>(
            this->container->webSocketService, this->container->plugin);
        this->container->timedEventHandlers->RegisterHandler(this->container->webSocketService);
        this->container->connectionEventHandlers->RegisterHandler(this->container->runwayService);
        this->container->routeService = std::make_shared<flightplan::RouteService>(this->container->plugin);
        this->container->messageService = std::make_shared<messages::MessageService>(
            this->container->plugin, this->container->webSocketService, this->container->flightPlanService,
            this->container->standService, this->container->routeService);
        this->container->messageHandlers->RegisterHandler(this->container->messageService);
        this->container->authenticationEventHandlers->RegisterHandler(this->container->webSocketService);

        Logger::Info(std::format("Loaded plugin version {}.", PLUGIN_VERSION));
    }

    void InitializePlugin::EuroScopeCleanup() {
        this->container->controllerEventHandlers->Clear();
        this->container->controllerEventHandlers.reset();
        this->container->flightPlanEventHandlers->Clear();
        this->container->flightPlanEventHandlers.reset();
        this->container->radarTargetEventHandlers->Clear();
        this->container->radarTargetEventHandlers.reset();
        this->container->airportRunwaysChangedEventHandlers->Clear();
        this->container->connectionEventHandlers->Clear();
        this->container->connectionEventHandlers.reset();
        this->container->airportRunwaysChangedEventHandlers.reset();
        this->container->timedEventHandlers->Clear();
        this->container->timedEventHandlers.reset();
        this->container->authenticationEventHandlers->Clear();
        this->container->authenticationEventHandlers.reset();
        this->container->messageHandlers->Clear();
        this->container->messageHandlers.reset();
        this->container->messageService.reset();
        this->container->filesystem.reset();
        this->container->webSocketService.reset();
        this->container->runwayService.reset();
        this->container->authenticationService.reset();
        this->container->plugin.reset();
        this->container->standService.reset();
        this->container->flightPlanService.reset();
        this->container->routeService.reset();
        this->container.reset();

        Logger::Info("Unloaded!");
    }
}
