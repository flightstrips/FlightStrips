// dllmain.cpp

#include "ExceptionHandling.h"
#include "InitializePlugin.h"
#include "version.h"

#ifndef CORE_API
#define CORE_API extern "C" __declspec(dllexport)
#define CORE_API_DIRECT __declspec(dllexport)
#endif

// Interface for EuroScope plugin loading
FlightStrips::InitializePlugin* plugin = nullptr;

HINSTANCE dllInstance;

[[maybe_unused]] auto __stdcall DllMain(HINSTANCE hinstance, [[maybe_unused]] DWORD dwReason, [[maybe_unused]] LPVOID lpvReserved) -> BOOL
{
    dllInstance = hinstance;
    return TRUE;
}

CORE_API auto LoadPlugin() -> EuroScopePlugIn::CPlugIn* {
    try {
        plugin = new FlightStrips::InitializePlugin;
        plugin->PostInit(dllInstance);

        return plugin->GetPlugin();
    } catch (...) {
        FlightStrips::exceptions::LogCurrentException("LoadPlugin");
        delete plugin;
        plugin = nullptr;
        return nullptr;
    }
}

CORE_API void UnloadPlugin() {
    if (plugin == nullptr) {
        return;
    }

    FlightStrips::exceptions::RunGuarded("UnloadPlugin::EuroScopeCleanup", [] {
        plugin->EuroScopeCleanup();
    });

    try {
        delete plugin;
    } catch (...) {
        FlightStrips::exceptions::LogCurrentException("UnloadPlugin::DeletePlugin");
    }

    plugin = nullptr;
}

CORE_API auto GetPluginVersion() -> const char * {
    return PLUGIN_VERSION;
}

[[maybe_unused]] CORE_API_DIRECT void EuroScopePlugInInit(EuroScopePlugIn::CPlugIn** ppPlugInInstance)
{
    if (ppPlugInInstance == nullptr) {
        return;
    }

    *ppPlugInInstance = LoadPlugin();
}

[[maybe_unused]] CORE_API_DIRECT void EuroScopePlugInExit(void)
{
    UnloadPlugin();
}
