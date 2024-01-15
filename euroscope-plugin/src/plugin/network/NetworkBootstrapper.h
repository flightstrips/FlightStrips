#pragma once

#include "bootstrap/Container.h"

namespace FlightStrips::network {
    class NetworkBootstrapper {
    public:
        static void Bootstrap(const std::shared_ptr<Container> &container);
    };
}
