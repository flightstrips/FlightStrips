//
// Created by fsr19 on 22/05/2023.
//

#include "StandsBootstrapper.h"

#include "Logger.hpp"
#include "configuration/AppConfig.h"
#include "filesystem/FileSystem.h"
#include "stands/StandService.h"
#include "plugin/FlightStripsPlugin.h"

namespace FlightStrips::stands {
    void StandsBootstrapper::Bootstrap(Container &container) {
        auto stands = LoadStands(*container.filesystem, *container.appConfig);
        container.plugin->Information(std::format("Loaded {} stands", stands.size()));
        Logger::Info(std::format("Loaded {} stands", stands.size()));
        container.standService = std::make_shared<StandService>(stands);
    }

    std::vector<Stand> StandsBootstrapper::LoadStands(filesystem::FileSystem &fileSystem, configuration::AppConfig &appConfig) {
        std::vector<Stand> stands;
        auto path = fileSystem.GetLocalFilePath(appConfig.GetStandsFile());
        std::ifstream filestream(path);

        if (!filestream.is_open()) {
            return stands;
        }

        std::string line;
        while (std::getline(filestream, line)) {
            if (!line.starts_with("STAND")) continue;

            stands.push_back(Stand::FromLine(line));
        }

        filestream.close();

        return stands;
    }
} // FlightStrips::stands