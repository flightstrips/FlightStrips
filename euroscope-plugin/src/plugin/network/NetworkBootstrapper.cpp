#include "NetworkBootstrapper.h"

#include "Server.h"
#include "NetworkService.h"
#include "handlers/FlightPlanEventHandlers.h"

namespace FlightStrips::network {
    void NetworkBootstrapper::Bootstrap(const std::shared_ptr<Container> &container) {
        container->server = std::make_shared<Server>(container);
        container->networkService = std::make_shared<NetworkService>(container->server, container->standService);
        container->flightPlanEventHandlers->RegisterHandler(container->networkService);
    }
}
