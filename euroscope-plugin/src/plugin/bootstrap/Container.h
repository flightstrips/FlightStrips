//
// Created by fsr19 on 19/05/2023.
//

#pragma once

namespace FlightStrips {
    class FlightStripsPlugin;
    namespace filesystem {
        class FileSystem;
    }
    namespace stands {
        class StandService;
    }
    namespace handlers {
        class FlightPlanEventHandlers;
        class ControllerEventHandlers;
        class TimedEventHandlers;
        class AirportRunwaysChangedEventHandlers;
        class RadarTargetEventHandlers;
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
        std::shared_ptr<handlers::ControllerEventHandlers> controllerEventHandlers;
        std::shared_ptr<handlers::FlightPlanEventHandlers> flightPlanEventHandlers;
        std::shared_ptr<handlers::RadarTargetEventHandlers> radarTargetEventHandlers;
        std::shared_ptr<handlers::TimedEventHandlers> timedEventHandlers;
        std::shared_ptr<handlers::AirportRunwaysChangedEventHandlers> airportRunwaysChangedEventHandlers;

        // flight plan
        std::shared_ptr<flightplan::FlightPlanService> flightPlanService;

        // Stands
        std::shared_ptr<stands::StandService> standService;
    };

}

