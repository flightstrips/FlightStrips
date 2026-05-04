#pragma once

#include <filesystem>

#include "ExceptionHandling.h"
#include "Loader.h"
#include "Logger.hpp"
#include "filesystem/FileSystem.h"
HINSTANCE loaderDllInstance;
HINSTANCE pluginDllInstance;

namespace {
    void ShowUpdateResultMessage(const FlightStrips::Loader::UpdateResult result) {
        switch (result) {
            case FlightStrips::Loader::UpdateResult::Failed:
                MessageBox(GetActiveWindow(), L"FlightStrips failed to update and will not start. Please contact the FlightStrips developers and inspect the logs.", L"FlightStrips Error", MB_OK | MB_ICONERROR);
                return;
            case FlightStrips::Loader::UpdateResult::UserRejected:
                MessageBox(GetActiveWindow(), L"FlightStrips update was cancelled. The plugin will not be loaded.", L"FlightStrips", MB_OK | MB_ICONWARNING);
                return;
            case FlightStrips::Loader::UpdateResult::Success:
                return;
        }
    }
}


[[maybe_unused]] auto __stdcall DllMain(HINSTANCE hinstance, [[maybe_unused]] DWORD dwReason, [[maybe_unused]] LPVOID lpvReserved) -> BOOL
{
    loaderDllInstance = hinstance;
    return TRUE;
}

[[maybe_unused]] void __declspec (dllexport)
EuroScopePlugInInit(EuroScopePlugIn::CPlugIn** ppPlugInInstance)
{
    if (ppPlugInInstance == nullptr) {
        return;
    }

    *ppPlugInInstance = nullptr;

    try {
        const auto fileSystem = std::make_shared<FlightStrips::filesystem::FileSystem>(loaderDllInstance);
        const auto logPath = fileSystem->GetLocalFilePath("flightstripsloader.log");
        Logger::Init(logPath.string(), LOG_INFO);
        FlightStrips::exceptions::InstallCrashHandlers("FlightStripsLoader");
        Logger::Info("Logger initialized!");

        const auto loader = FlightStrips::Loader{fileSystem};

        if (const auto latestVersion = FlightStrips::Loader::GetLatestPluginVersion(); loader.ShouldUpdate(latestVersion)) {
            const auto updateResult = loader.Update(latestVersion);

            if (updateResult != FlightStrips::Loader::UpdateResult::Success) {
                ShowUpdateResultMessage(updateResult);
                pluginDllInstance = nullptr;
                Logger::Shutdown();
                return;
            }
        }

        pluginDllInstance = loader.LoadPluginDll();
        if (!pluginDllInstance) return;

        const auto pluginPtr = FlightStrips::Loader::GetPluginInstance(pluginDllInstance);
        if (pluginPtr == nullptr) {
            FlightStrips::Loader::UnloadPluginDll(pluginDllInstance);
            pluginDllInstance = nullptr;
            return;
        }

        *ppPlugInInstance = pluginPtr;
    } catch (...) {
        FlightStrips::exceptions::LogCurrentException("Loader::EuroScopePlugInInit");

        if (pluginDllInstance != nullptr) {
            try {
                FlightStrips::Loader::UnloadPluginDll(pluginDllInstance);
            } catch (...) {
                FlightStrips::exceptions::LogCurrentException("Loader::EuroScopePlugInInit::UnloadPluginDll");
            }

            pluginDllInstance = nullptr;
        }

        MessageBox(GetActiveWindow(), L"FlightStrips failed to start. Please contact the FlightStrips developers and inspect the logs.", L"FlightStrips Error", MB_OK | MB_ICONERROR);
        Logger::Shutdown();
    }
}

[[maybe_unused]] void __declspec (dllexport)
EuroScopePlugInExit(void)
{
    try {
        FlightStrips::Loader::UnloadPluginDll(pluginDllInstance);
    } catch (...) {
        FlightStrips::exceptions::LogCurrentException("Loader::EuroScopePlugInExit");
    }

    pluginDllInstance = nullptr;
    Logger::Shutdown();
}
