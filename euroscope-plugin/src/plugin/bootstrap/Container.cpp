//
// Created by fsr19 on 19/05/2023.
//

#include "Container.h"
#include "plugin/FlightStripsPlugin.h"
#include "filesystem/FileSystem.h"
#include "stands/StandService.h"
#include "network/Server.h"
#include "network/NetworkService.h"
#include "handlers/FlightPlanEventHandlers.h"

namespace FlightStrips {
    Container::Container() = default;
    Container::~Container() = default;
    Container::Container(Container&&) noexcept = default;
    auto Container::operator=(Container&&) noexcept -> Container& = default;
}

