#include "CdmReadyTrigger.h"

namespace FlightStrips::messages {
    namespace {
        constexpr auto kExternalCdmPluginName = "CDM Plugin";
        constexpr auto kReadyTobtMenuItemCode = 0;
        constexpr auto kReadyTobtFunctionId = 114;
    }

    bool CdmReadyTrigger::Trigger(TagFunctionInvoker& tagFunctionInvoker, const std::string& callsign) {
        if (callsign.empty()) {
            return false;
        }

        if (!tagFunctionInvoker.SelectActiveAircraft(callsign)) {
            return false;
        }

        tagFunctionInvoker.InvokeTagFunction(
            "",
            kExternalCdmPluginName,
            kReadyTobtMenuItemCode,
            "",
            kExternalCdmPluginName,
            kReadyTobtFunctionId);
        return true;
    }
}
