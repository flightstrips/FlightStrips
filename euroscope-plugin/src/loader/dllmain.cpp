#pragma once

#include <filesystem>

#include "Loader.h"
#include "Logger.hpp"
#include "filesystem/FileSystem.h"
HINSTANCE loaderDllInstance;
HINSTANCE pluginDllInstance;


[[maybe_unused]] auto __stdcall DllMain(HINSTANCE hinstance, [[maybe_unused]] DWORD dwReason, [[maybe_unused]] LPVOID lpvReserved) -> BOOL
{
    loaderDllInstance = hinstance;
    return TRUE;
}

[[maybe_unused]] void __declspec (dllexport)
EuroScopePlugInInit(EuroScopePlugIn::CPlugIn** ppPlugInInstance)
{
    const auto fileSystem = std::make_shared<FlightStrips::filesystem::FileSystem>(loaderDllInstance);;
    const auto logPath = fileSystem->GetLocalFilePath("flightstripsloader.log");
    Logger::Init(logPath.string(), LOG_INFO);
    Logger::Info("Logger initialized!");

    const auto loader = FlightStrips::Loader{fileSystem};

    if (const auto latestVersion = FlightStrips::Loader::GetLatestPluginVersion(); loader.ShouldUpdate(latestVersion)) {
        if (!loader.Update(latestVersion)) {
            MessageBox(GetActiveWindow(), L"Failed to update FlightStrips plugin! Please contact the FlightStrips developers!", L"FlightStrips Error", MB_OK | MB_ICONERROR);
        }
    }

    loaderDllInstance = loader.LoadPluginDll();
    if (!loaderDllInstance) return;
    const auto pluginPtr = FlightStrips::Loader::GetPluginInstance(loaderDllInstance);
    if (pluginPtr == nullptr) {
        FlightStrips::Loader::UnloadPluginDll(loaderDllInstance);
        return;
    }
    *ppPlugInInstance = pluginPtr;
}

[[maybe_unused]] void __declspec (dllexport)
EuroScopePlugInExit(void)
{
    Logger::Shutdown();
    FlightStrips::Loader::UnloadPluginDll(pluginDllInstance);
}