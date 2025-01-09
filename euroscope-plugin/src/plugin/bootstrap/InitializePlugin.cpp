#include "InitializePlugin.h"
#include "plugin/FlightStripsPlugin.h"
#include "filesystem/FileSystem.h"
#include "stands/StandsBootstrapper.h"
#include "euroscope/EuroScopePlugIn.h"
#include "handlers/ControllerEventHandlers.h"
#include "handlers/TimedEventHandlers.h"
#include "handlers/AirportRunwaysChangedEventHandlers.h"

namespace FlightStrips {
    auto InitializePlugin::GetPlugin() -> EuroScopePlugIn::CPlugIn* {
        return static_cast<FlightStripsPlugin*>(this->container->plugin.get());
    }

    void InitializePlugin::PostInit(HINSTANCE dllInstance) {
        this->container = std::make_shared<Container>();
        this->container->controllerEventHandlers = std::make_shared<handlers::ControllerEventHandlers>();
        this->container->flightPlanEventHandlers = std::make_shared<handlers::FlightPlanEventHandlers>();
        this->container->radarTargetEventHandlers = std::make_shared<handlers::RadarTargetEventHandlers>();
        this->container->timedEventHandlers = std::make_shared<handlers::TimedEventHandlers>();
        this->container->airportRunwaysChangedEventHandlers = std::make_shared<handlers::AirportRunwaysChangedEventHandlers>();
        this->container->filesystem = std::make_unique<filesystem::FileSystem>(dllInstance);
        stands::StandsBootstrapper::Bootstrap(*this->container);
        //flightplan::FlightPlanBootstrapper::Bootstrap(*this->container);

        this->container->plugin = std::make_shared<FlightStripsPlugin>(this->container->flightPlanEventHandlers, this->container->radarTargetEventHandlers, this->container->controllerEventHandlers, this->container->timedEventHandlers, this->container->airportRunwaysChangedEventHandlers);

        this->container->plugin->Information("Initialized");
    }

    void InitializePlugin::EuroScopeCleanup() {
        this->container->controllerEventHandlers->Clear();
        this->container->controllerEventHandlers.reset();
        this->container->flightPlanEventHandlers->Clear();
        this->container->flightPlanEventHandlers.reset();
        this->container->radarTargetEventHandlers->Clear();
        this->container->radarTargetEventHandlers.reset();
        this->container->airportRunwaysChangedEventHandlers->Clear();
        this->container->airportRunwaysChangedEventHandlers.reset();
        this->container->timedEventHandlers->Clear();
        this->container->timedEventHandlers.reset();
        this->container->filesystem.reset();
        this->container->plugin.reset();
        this->container->standService.reset();
        this->container->flightPlanService.reset();
        this->container.reset();
    }

}
