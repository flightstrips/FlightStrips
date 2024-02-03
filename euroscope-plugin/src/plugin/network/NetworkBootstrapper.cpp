#include "NetworkBootstrapper.h"

#include "Server.h"
#include "NetworkService.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "handlers/ControllerEventHandlers.h"

namespace FlightStrips::network {
    void NetworkBootstrapper::Bootstrap(Container &container) {
        container.server = std::make_shared<Server>(container);
        container.networkService = std::make_shared<NetworkService>(container.server, container.standService);
        container.flightPlanEventHandlers->RegisterHandler(container.networkService);
        container.controllerEventHandlers->RegisterHandler(container.networkService);
    }
}
