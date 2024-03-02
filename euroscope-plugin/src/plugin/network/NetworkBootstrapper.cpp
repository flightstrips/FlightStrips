#include "NetworkBootstrapper.h"

#include "NetworkService.h"
#include "handlers/FlightPlanEventHandlers.h"

namespace FlightStrips::network {
    void NetworkBootstrapper::Bootstrap(Container &container) {
        container.channel = grpc::CreateChannel("localhost:50051", grpc::InsecureChannelCredentials());
        container.networkService = std::make_shared<NetworkService>(container.plugin, container.channel);
        container.flightPlanEventHandlers->RegisterHandler(container.networkService);
        container.controllerEventHandlers->RegisterHandler(container.networkService);
        container.radarTargetEventHandlers->RegisterHandler(container.networkService);
        container.timedEventHandlers->RegisterHandler(container.networkService);
    }
}
