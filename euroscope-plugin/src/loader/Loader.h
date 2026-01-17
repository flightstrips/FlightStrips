#pragma once

#include "filesystem/FileSystem.h"

namespace FlightStrips {
    class Loader {

    public:
        explicit Loader(const std::shared_ptr<filesystem::FileSystem>& fileSystem) : fileSystem(fileSystem) {}

        [[nodiscard]] bool ShouldUpdate(const std::string &latestVersion) const;
        [[nodiscard]] bool Update(std::string latestVersion) const;
        [[nodiscard]] HINSTANCE LoadPluginDll() const;

        static EuroScopePlugIn::CPlugIn* GetPluginInstance(HINSTANCE pluginInstance);
        static void UnloadPluginDll(HINSTANCE pluginInstance);
        static std::string GetLatestPluginVersion();
    private:
        typedef EuroScopePlugIn::CPlugIn*(CALLBACK* LOADPLUGINLIBRARY)();
        typedef void(CALLBACK* UNLOADPLUGINLIBRARY)();
        typedef const char* (CALLBACK* GETPLUGINVERSION)();


        const std::string pluginDLL = "FlightStripsPluginCore.dll";
        const std::string pluginDLLOld = "FlightStripsPluginCore.dll.old";
        const std::string config = "flightstrips_config.ini";
        const std::string configOld = "flightstrips_config.ini.old";

        std::shared_ptr<filesystem::FileSystem> fileSystem;
    };
}
