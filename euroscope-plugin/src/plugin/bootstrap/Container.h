//
// Created by fsr19 on 19/05/2023.
//

#pragma once

#include <memory>
#include <functional>

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
        std::unique_ptr<FlightStripsPlugin> plugin;

        // FileSystem
        std::unique_ptr<filesystem::FileSystem> filesystem;

        // event collections
        std::shared_ptr<handlers::FlightPlanEventHandlers> flightPlanEventHandlers;

        // network
        std::shared_ptr<network::Server> server;
        std::shared_ptr<network::NetworkService> networkService;

        // Stands
        std::unique_ptr<stands::StandService> standService;
    };

}

