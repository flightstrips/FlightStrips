
#pragma once

namespace FlightStrips {
    struct Sid {
        std::string name;
        std::string runway;
    };

    class FlightStripsPluginInterface {
    public:
        virtual std::vector<Sid> GetSids(const std::string& airport) = 0;
        virtual ~FlightStripsPluginInterface() = default;
    };
}
