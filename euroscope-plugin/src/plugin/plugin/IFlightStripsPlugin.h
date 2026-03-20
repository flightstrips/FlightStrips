#pragma once
#include <string>

namespace FlightStrips {
    enum ConnectionType {
        CONNECTION_TYPE_NO               = 0,
        CONNECTION_TYPE_DIRECT           = 1,
        CONNECTION_TYPE_VIA_PROXY        = 2,
        CONNECTION_TYPE_SIMULATOR_SERVER = 3,
        CONNECTION_TYPE_PLAYBACK         = 4,
        CONNECTION_TYPE_SIMULATOR_CLIENT = 5,
        CONNECTION_TYPE_SWEATBOX         = 6
    };

    struct ConnectionState {
        int range;
        ConnectionType connection_type;
        std::string primary_frequency;
        std::string callsign;
        std::string relevant_airport;
    };

    class IFlightStripsPlugin {
    public:
        virtual ~IFlightStripsPlugin() = default;
        virtual ConnectionState& GetConnectionState() = 0;
    };
}
