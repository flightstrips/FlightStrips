#pragma once

#include "bootstrap/Container.h"
#include "Stand.h"

namespace FlightStrips::stands {

    class StandsBootstrapper {
    public:
        static void Bootstrap(Container& container);

    private:
        static std::vector<Stand> LoadStands(filesystem::FileSystem& fileSystem, configuration::AppConfig& appConfig);
    };

} // FlightStrips::stands
