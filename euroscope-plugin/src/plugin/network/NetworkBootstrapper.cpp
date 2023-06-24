#include "NetworkBootstrapper.h"

#include "Server.h"
#include "NetworkService.h"
#include "handlers/FlightPlanEventHandlers.h"

namespace FlightStrips::network {
    void NetworkBootstrapper::Bootstrap(Container &container) {
        container.server = std::make_shared<Server>();
        container.networkService = std::make_shared<NetworkService>(container.server);
        container.flightPlanEventHandlers->RegisterHandler(container.networkService);
    }
}
