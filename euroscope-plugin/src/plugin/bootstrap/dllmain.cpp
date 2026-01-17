// dllmain.cpp

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
    plugin = new FlightStrips::InitializePlugin;
    plugin->PostInit(dllInstance);

    return plugin->GetPlugin();
}

CORE_API void UnloadPlugin() {
    plugin->EuroScopeCleanup();
    delete plugin;
}

CORE_API auto GetPluginVersion() -> const char * {
    return PLUGIN_VERSION;
}

[[maybe_unused]] CORE_API_DIRECT void EuroScopePlugInInit(EuroScopePlugIn::CPlugIn** ppPlugInInstance)
{
    *ppPlugInInstance = LoadPlugin();
}

[[maybe_unused]] CORE_API_DIRECT void EuroScopePlugInExit(void)
{
    plugin->EuroScopeCleanup();
    delete plugin;
}