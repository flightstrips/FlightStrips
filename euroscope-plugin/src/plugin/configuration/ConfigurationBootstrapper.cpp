//
// Created by fsr19 on 10/01/2025.
//

#include "ConfigurationBootstrapper.h"

#include "AppConfig.h"
#include "Logger.h"
#include "UserConfig.h"
#include "filesystem/FileSystem.h"


namespace FilghtStrips::configuration {
    void ConfigurationBootstrapper::Bootstrap(FlightStrips::Container &container) {
        const auto configPath = container.filesystem->GetLocalFilePath("flightstrips_config.ini");
        const auto userPath = container.filesystem->GetLocalFilePath("flightstrips_user.ini");

        container.appConfig = std::make_shared<FlightStrips::configuration::AppConfig>(configPath.string());
        container.userConfig = std::make_shared<FlightStrips::configuration::UserConfig>(userPath.string());

    }
} // configuration
// FilghtStrips
