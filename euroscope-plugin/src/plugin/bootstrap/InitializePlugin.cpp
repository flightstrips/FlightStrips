//
// Created by fsr19 on 19/05/2023.
//
#include "InitializePlugin.h"
#include "plugin/FlightStripsPlugin.h"
#include "filesystem/FileSystem.h"
#include "stands/StandsBootstrapper.h"
#include "euroscope/EuroScopePlugIn.h"
#include "network/NetworkBootstrapper.h"
#include "flightplan/FlightPlanBootstrapper.h"

namespace FlightStrips {
    auto InitializePlugin::GetPlugin() -> EuroScopePlugIn::CPlugIn* {
        return static_cast<FlightStripsPlugin*>(this->container->plugin.get());
    }

    void InitializePlugin::PostInit(HINSTANCE dllInstance) {
        this->container = std::make_shared<Container>();
        this->container->flightPlanEventHandlers = std::make_shared<handlers::FlightPlanEventHandlers>();
        this->container->radarTargetEventHandlers = std::make_shared<handlers::RadarTargetEventHandlers>();
        network::NetworkBootstrapper::Bootstrap(*this->container);
        flightplan::FlightPlanBootstrapper::Bootstrap(*this->container);
        this->container->filesystem = std::make_unique<filesystem::FileSystem>(dllInstance);
        stands::StandsBootstrapper::Bootstrap(*this->container);

        this->container->plugin = std::make_unique<FlightStripsPlugin>(this->container->flightPlanEventHandlers, this->container->radarTargetEventHandlers, this->container->networkService);

        this->container->plugin->Information("Initialized");
    }

    void InitializePlugin::EuroScopeCleanup() {
    }

}
