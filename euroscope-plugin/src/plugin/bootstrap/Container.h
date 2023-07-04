//
// Created by fsr19 on 19/05/2023.
//

#pragma once

#include <memory>
#include <functional>
#include "handlers/RadarTargetEventHandlers.h"

namespace FlightStrips {
    class FlightStripsPlugin;
    namespace filesystem {
        class FileSystem;
    }
    namespace stands {
        class StandService;
    }
    namespace network {
        class Server;
        class NetworkService;
    }
    namespace handlers {
        class FlightPlanEventHandlers;
    }
    namespace flightplan {
        class FlightPlanService;
    }
}

namespace EuroScopePlugIn {
    class CFlightPlan;
}

namespace FlightStrips {
    using Container = struct Container {
        Container();
        ~Container();
        Container(const Container&) = delete;
        Container(Container&&) noexcept;
        auto operator=(const Container&) -> Container& = delete;
        auto operator=(Container&&) noexcept -> Container&;

        // The plugin
        std::shared_ptr<FlightStripsPlugin> plugin;

        // FileSystem
        std::unique_ptr<filesystem::FileSystem> filesystem;

        // event collections
        std::shared_ptr<handlers::FlightPlanEventHandlers> flightPlanEventHandlers;
        std::shared_ptr<handlers::RadarTargetEventHandlers> radarTargetEventHandlers;

        // network
        std::shared_ptr<network::Server> server;
        std::shared_ptr<network::NetworkService> networkService;

        // flight plan
        std::shared_ptr<flightplan::FlightPlanService> flightPlanService;

        // Stands
        std::unique_ptr<stands::StandService> standService;
    };

}

