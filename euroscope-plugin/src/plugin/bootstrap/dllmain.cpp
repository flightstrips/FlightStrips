// dllmain.cpp

#include "InitializePlugin.h"

// Interface for EuroScope plugin loading
FlightStrips::InitializePlugin* plugin = nullptr;

HINSTANCE dllInstance;

[[maybe_unused]] auto DllMain(HINSTANCE hinstance, [[maybe_unused]] DWORD dwReason, [[maybe_unused]] LPVOID lpvReserved) -> BOOL
{
    dllInstance = hinstance;
    return TRUE;
}

[[maybe_unused]] void __declspec (dllexport)
EuroScopePlugInInit(EuroScopePlugIn::CPlugIn** ppPlugInInstance)
{
    plugin = new FlightStrips::InitializePlugin;
    plugin->PostInit(dllInstance);

    *ppPlugInInstance = plugin->GetPlugin();
}

[[maybe_unused]] void __declspec (dllexport)
EuroScopePlugInExit(void)
{
    plugin->EuroScopeCleanup();
    delete plugin;
}