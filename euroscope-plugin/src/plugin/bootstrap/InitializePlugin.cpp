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
#include "handlers/ConnectionEventHandlers.h"
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
        this->container->airportRunwaysChangedEventHandlers = std::make_shared<
            handlers::AirportRunwaysChangedEventHandlers>();
        stands::StandsBootstrapper::Bootstrap(*this->container);
        //flightplan::FlightPlanBootstrapper::Bootstrap(*this->container);

        this->container->authenticationService = std::make_shared<authentication::AuthenticationService>(
            this->container->appConfig, this->container->userConfig);
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
            this->container->connectionEventHandlers);
        this->container->runwayService = std::make_shared<runway::RunwayService>(this->container->webSocketService, this->container->plugin);
        this->container->timedEventHandlers->RegisterHandler(this->container->webSocketService);
        this->container->connectionEventHandlers->RegisterHandler(this->container->runwayService);

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
        this->container->filesystem.reset();
        this->container->webSocketService.reset();
        this->container->runwayService.reset();
        this->container->authenticationService.reset();
        this->container->plugin.reset();
        this->container->standService.reset();
        this->container->flightPlanService.reset();
        this->container.reset();

        Logger::Info("Unloaded!");
    }
}
