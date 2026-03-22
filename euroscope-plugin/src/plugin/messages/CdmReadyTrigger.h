#pragma once

#include <string>

namespace FlightStrips::messages {
    class TagFunctionInvoker {
    public:
        virtual ~TagFunctionInvoker() = default;

        virtual bool SelectActiveAircraft(const std::string& callsign) = 0;

        virtual void InvokeTagFunction(
            const std::string& itemString,
            const std::string& menuName,
            int menuItemCode,
            const std::string& parameter,
            const std::string& targetPluginName,
            int targetFunctionId) = 0;
    };

    class CdmReadyTrigger final {
    public:
        static bool Trigger(TagFunctionInvoker& tagFunctionInvoker, const std::string& callsign);
    };
}
