#include "FlightPlanBootstrapper.h"

#include "FlightPlanService.h"
#include "handlers/RadarTargetEventHandlers.h"

namespace FlightStrips::flightplan {
    void FlightPlanBootstrapper::Bootstrap(Container &container) {
        container.flightPlanService = std::make_shared<FlightPlanService>(container.webSocketService, container.plugin, container.standService);
        container.radarTargetEventHandlers->RegisterHandler(container.flightPlanService);
        container.flightPlanEventHandlers->RegisterHandler(container.flightPlanService);
    }
}
