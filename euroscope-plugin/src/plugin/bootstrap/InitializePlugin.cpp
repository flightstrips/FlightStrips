#include "InitializePlugin.h"

#include "ExceptionHandling.h"
#include "Logger.hpp"
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
#include "tag_items/CdmStateHandler.h"
#include "tag_items/DeIceHandler.h"
#include "tag_items/ClearanceStatusHandler.h"
#include "graphics/PdcClearancePopupState.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips {
    auto InitializePlugin::GetPlugin() -> EuroScopePlugIn::CPlugIn * {
        return this->container->plugin.get();
    }

    void InitializePlugin::PostInit(HINSTANCE dllInstance) {
        this->container = std::make_shared<Container>();
        this->container->filesystem = std::make_unique<filesystem::FileSystem>(dllInstance);
        FilghtStrips::configuration::ConfigurationBootstrapper::Bootstrap(*this->container);
        const auto logLevel = Logger::GetLevelFromString(this->container->appConfig->GetLogLevel());
        const auto logPath = this->container->filesystem->GetLocalFilePath("flightstrips.log").string();
        Logger::Init(logPath, logLevel);
        exceptions::InstallCrashHandlers("FlightStripsPluginCore");
        Logger::Debug("Logger initialized and loaded configuration!");

        this->container->pdcPopup = std::make_shared<graphics::PdcClearancePopupState>();

        this->container->connectionEventHandlers = std::make_shared<handlers::ConnectionEventHandlers>();
        this->container->controllerEventHandlers = std::make_shared<handlers::ControllerEventHandlers>();
        this->container->flightPlanEventHandlers = std::make_shared<handlers::FlightPlanEventHandlers>();
        this->container->radarTargetEventHandlers = std::make_shared<handlers::RadarTargetEventHandlers>();
        this->container->timedEventHandlers = std::make_shared<handlers::TimedEventHandlers>();
        this->container->messageHandlers = std::make_shared<handlers::MessageHandlers>();
        this->container->authenticationEventHandlers = std::make_shared<handlers::AuthenticationEventHandlers>();
        this->container->airportRunwaysChangedEventHandlers = std::make_shared<
            handlers::AirportRunwaysChangedEventHandlers>();
        this->container->tagItemHandlers = std::make_shared<TagItems::TagItemHandlers>();
        stands::StandsBootstrapper::Bootstrap(*this->container);

        // Tag items
        this->container->deIceHandler = std::make_shared<TagItems::DeIceHandler>(this->container->standService, this->container->appConfig);
        this->container->tagItemHandlers->RegisterHandler(this->container->deIceHandler, TAG_ITEM_DEICING_DESIGNATOR);
        this->container->flightPlanEventHandlers->RegisterHandler(this->container->deIceHandler);

        this->container->authenticationService = std::make_shared<authentication::AuthenticationService>(
            this->container->appConfig, this->container->userConfig, this->container->authenticationEventHandlers);
        this->container->timedEventHandlers->RegisterHandler(this->container->authenticationService);
        this->container->plugin = std::make_shared<FlightStripsPlugin>(this->container->flightPlanEventHandlers,
                                                                       this->container->radarTargetEventHandlers,
                                                                       this->container->controllerEventHandlers,
                                                                       this->container->timedEventHandlers,
                                                                       this->container->
                                                                       airportRunwaysChangedEventHandlers,
                                                                       this->container,
                                                                       this->container->appConfig,
                                                                       this->container->tagItemHandlers);
        this->container->webSocketService = std::make_shared<websocket::WebSocketService>(
            this->container->appConfig->GetBaseUrl(), this->container->appConfig->GetApiEnabled(),
            this->container->authenticationService, this->container->plugin,
            this->container->connectionEventHandlers, this->container->messageHandlers);
        flightplan::FlightPlanBootstrapper::Bootstrap(*this->container);
        this->container->deIceHandler->SetFlightPlanService(this->container->flightPlanService);
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Eobt),
            TAG_ITEM_CDM_EOBT
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Phase),
            TAG_ITEM_CDM_PHASE
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Tobt),
            TAG_ITEM_CDM_TOBT
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::ReqTobt),
            TAG_ITEM_CDM_REQ_TOBT
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Tsat),
            TAG_ITEM_CDM_TSAT
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::TsatTobtDiff),
            TAG_ITEM_CDM_TSAT_TOBT_DIFF
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Ttg),
            TAG_ITEM_CDM_TTG
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Ttot),
            TAG_ITEM_CDM_TTOT
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Ctot),
            TAG_ITEM_CDM_CTOT
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::FlowMessage),
            TAG_ITEM_CDM_FLOW_MESSAGE
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Status),
            TAG_ITEM_CDM_STATUS
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Status),
            TAG_ITEM_CDM_NETWORK_STATUS
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::TobtConfirmedBy),
            TAG_ITEM_CDM_TOBT_CONFIRMED_BY
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Asrt),
            TAG_ITEM_CDM_ASRT
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::ReadyStartup),
            TAG_ITEM_CDM_READY_STARTUP
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Tsac),
            TAG_ITEM_CDM_TSAC
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::CdmStateHandler>(this->container->flightPlanService, TagItems::CdmStateHandler::Field::Asat),
            TAG_ITEM_CDM_ASAT
        );
        this->container->tagItemHandlers->RegisterHandler(
            std::make_shared<TagItems::ClearanceStatusHandler>(this->container->flightPlanService),
            TAG_ITEM_CLEARANCE_STATUS
        );
        this->container->controllerService = std::make_shared<controller::ControllerService>(
            this->container->webSocketService);
        this->container->controllerEventHandlers->RegisterHandler(this->container->controllerService);
        this->container->runwayService = std::make_shared<runway::RunwayService>(
            this->container->webSocketService, this->container->plugin);
        this->container->timedEventHandlers->RegisterHandler(this->container->webSocketService);
        this->container->connectionEventHandlers->RegisterHandler(this->container->runwayService);
        this->container->airportRunwaysChangedEventHandlers->RegisterHandler(this->container->runwayService);
        this->container->routeService = std::make_shared<flightplan::RouteService>(this->container->plugin);
        this->container->messageService = std::make_shared<messages::MessageService>(
            this->container->plugin, this->container->webSocketService, this->container->flightPlanService,
            this->container->standService, this->container->routeService, this->container->runwayService);
        this->container->messageHandlers->RegisterHandler(this->container->messageService);
        this->container->authenticationEventHandlers->RegisterHandler(this->container->webSocketService);

        this->container->plugin->Information(std::format("Loaded plugin version {}.", PLUGIN_VERSION));
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
        this->container->tagItemHandlers->Clear();
        this->container->tagItemHandlers.reset();
        this->container->deIceHandler.reset();
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
