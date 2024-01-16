#include "InitializePlugin.h"
#include "plugin/FlightStripsPlugin.h"
#include "filesystem/FileSystem.h"
#include "stands/StandsBootstrapper.h"
#include "euroscope/EuroScopePlugIn.h"
#include "network/NetworkBootstrapper.h"
#include "flightplan/FlightPlanBootstrapper.h"
#include "handlers/ControllerEventHandlers.h"

namespace FlightStrips {
    auto InitializePlugin::GetPlugin() -> EuroScopePlugIn::CPlugIn* {
        return static_cast<FlightStripsPlugin*>(this->container->plugin.get());
    }

    void InitializePlugin::PostInit(HINSTANCE dllInstance) {
        this->container = std::make_shared<Container>();
        this->container->controllerEventHandlers = std::make_shared<handlers::ControllerEventHandlers>();
        this->container->flightPlanEventHandlers = std::make_shared<handlers::FlightPlanEventHandlers>();
        this->container->radarTargetEventHandlers = std::make_shared<handlers::RadarTargetEventHandlers>();
        this->container->filesystem = std::make_unique<filesystem::FileSystem>(dllInstance);
        stands::StandsBootstrapper::Bootstrap(*this->container);
        flightplan::FlightPlanBootstrapper::Bootstrap(*this->container);
        network::NetworkBootstrapper::Bootstrap(*this->container);

        this->container->plugin = std::make_shared<FlightStripsPlugin>(this->container->flightPlanEventHandlers, this->container->radarTargetEventHandlers, this->container->controllerEventHandlers, this->container->networkService);

        this->container->plugin->Information("Initialized");
    }

    void InitializePlugin::EuroScopeCleanup() {
    }

}
