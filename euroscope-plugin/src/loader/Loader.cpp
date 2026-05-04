#include "Loader.h"

#include "Logger.hpp"
#include "http/Http.h"

constexpr static auto downloadUrlTemplate = "https://github.com/flightstrips/FlightStrips/releases/download/plugin%2Fv{}/{}";
constexpr static auto versionFileUrl = "https://raw.githubusercontent.com/flightstrips/FlightStrips/refs/heads/main/plugin-release-version.txt";

bool FlightStrips::Loader::ShouldUpdate(const std::string &latestVersion) const {
    if (!fileSystem->DoesExist(pluginDLL)) return true;

    const auto corePath = fileSystem->GetLocalFilePath(pluginDLL);

    auto pluginDllInstance = LoadLibrary(corePath.c_str());
    if (!pluginDllInstance) {
        Logger::Error("Failed to load plugin dll {}", corePath.string());
        return true;
    }

    const auto getPluginVersion = reinterpret_cast<GETPLUGINVERSION>(GetProcAddress(pluginDllInstance, "GetPluginVersion"));
    if (!getPluginVersion) {
        FreeLibrary(pluginDllInstance);
        Logger::Error("Failed to load plugin entry point");
        return true;
    }

    const auto currentVersion = std::string(getPluginVersion());
    FreeLibrary(pluginDllInstance);
    return currentVersion != latestVersion;
}

FlightStrips::Loader::UpdateResult FlightStrips::Loader::Update(const std::string &latestVersion) const {
    Logger::Info("Updating plugin to version {}", latestVersion);

    // Show dialog FIRST — no file operations until the user accepts
    if (const auto result = MessageBox(GetActiveWindow(),
        L"Updating the FlightStrips plugin. This will download the latest version of FlightStrips.\r\n\r\nSelect OK to continue.",
        L"FlightStrips", MB_OKCANCEL | MB_ICONINFORMATION); result != IDOK) {
        Logger::Info("User rejected plugin update to version {}", latestVersion);
        return UpdateResult::UserRejected;
    }

    bool pluginDllBackedUp = false;
    bool configBackedUp = false;

    const auto backupFile = [&](const std::string& currentFile, const std::string& backupFile, bool& backupCreated) {
        backupCreated = false;
        if (!fileSystem->DoesExist(currentFile)) {
            return true;
        }

        fileSystem->DeleteFileIfExists(backupFile);
        if (!fileSystem->Rename(currentFile, backupFile)) {
            Logger::Error("Failed to back up {} to {}", currentFile, backupFile);
            return false;
        }

        backupCreated = true;
        return true;
    };

    const auto restoreBackups = [&] {
        if (pluginDllBackedUp) {
            fileSystem->DeleteFileIfExists(pluginDLL);
            if (!fileSystem->Rename(pluginDLLOld, pluginDLL)) {
                Logger::Error("Failed to restore {}", pluginDLL);
            }
        }

        if (configBackedUp) {
            fileSystem->DeleteFileIfExists(config);
            if (!fileSystem->Rename(configOld, config)) {
                Logger::Error("Failed to restore {}", config);
            }
        }
    };

    if (!backupFile(pluginDLL, pluginDLLOld, pluginDllBackedUp)) {
        return UpdateResult::Failed;
    }

    if (!backupFile(config, configOld, configBackedUp)) {
        restoreBackups();
        return UpdateResult::Failed;
    }

    // Download new DLL
    auto downloadUrl = std::format(downloadUrlTemplate, latestVersion, pluginDLL);
    if (!http::Http::DownloadFile(downloadUrl, fileSystem->GetLocalFilePath(pluginDLL).string())) {
        Logger::Error("Failed to download plugin dll from {}", downloadUrl);
        fileSystem->DeleteFileIfExists(pluginDLL);
        restoreBackups();
        return UpdateResult::Failed;
    }

    // Download new config
    downloadUrl = std::format(downloadUrlTemplate, latestVersion, config);
    if (!http::Http::DownloadFile(downloadUrl, fileSystem->GetLocalFilePath(config).string())) {
        Logger::Error("Failed to download config file from {}", downloadUrl);
        fileSystem->DeleteFileIfExists(pluginDLL);
        fileSystem->DeleteFileIfExists(config);
        restoreBackups();
        return UpdateResult::Failed;
    }

    Logger::Info("Successfully updated plugin to version {}", latestVersion);
    return UpdateResult::Success;
}

HINSTANCE FlightStrips::Loader::LoadPluginDll() const {
    if (!fileSystem->DoesExist(pluginDLL)) {
        Logger::Error("Plugin dll {} does not exist", pluginDLL);
        return nullptr;
    }

    const auto corePath = fileSystem->GetLocalFilePath(pluginDLL);

    return LoadLibrary(corePath.c_str());
}

EuroScopePlugIn::CPlugIn * FlightStrips::Loader::GetPluginInstance(HINSTANCE pluginInstance) {
    const auto loadPlugin = reinterpret_cast<LOADPLUGINLIBRARY>(GetProcAddress(pluginInstance, "LoadPlugin"));
    if (!loadPlugin) {
        Logger::Error("Failed to load plugin entry point");
        return nullptr;
    }
    return loadPlugin();
}

void FlightStrips::Loader::UnloadPluginDll(HINSTANCE pluginInstance) {
    if (!pluginInstance) return;
    if (const auto unloadPlugin = reinterpret_cast<UNLOADPLUGINLIBRARY>(GetProcAddress(pluginInstance, "UnloadPlugin")); !unloadPlugin) {
        Logger::Error("Failed to load plugin entry point");

    } else {
        unloadPlugin();
    }
    FreeLibrary(pluginInstance);
}

std::string FlightStrips::Loader::GetLatestPluginVersion() {
    auto [status_code, content] = http::Http::Get(versionFileUrl);

    if (status_code != 200) return {};

    std::erase_if(content, ::isspace);
    return content;
}
