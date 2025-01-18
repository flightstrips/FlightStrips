//
// Created by fsr19 on 19/05/2023.
//

#include "Container.h"
#include "plugin/FlightStripsPlugin.h"
#include "filesystem/FileSystem.h"
#include "stands/StandService.h"
#include "handlers/FlightPlanEventHandlers.h"
#include "handlers/ControllerEventHandlers.h"
#include "handlers/TimedEventHandlers.h"
#include "handlers/RadarTargetEventHandlers.h"
#include"flightplan/FlightPlanService.h"
#include "configuration/AppConfig.h"
#include "configuration/UserConfig.h"
#include "authentication/AuthenticationService.h"
#include "websocket/WebSocketService.h"

namespace FlightStrips {
    Container::Container() = default;
    Container::~Container() = default;
    Container::Container(Container&&) noexcept = default;
    auto Container::operator=(Container&&) noexcept -> Container& = default;
}

