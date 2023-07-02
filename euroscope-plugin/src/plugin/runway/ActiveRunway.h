#pragma once

#include <string>

namespace FlightStrips::runway {
    struct ActiveRunway {
        std::string name;
        bool isDeparture;
    };

}