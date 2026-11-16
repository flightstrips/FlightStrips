#pragma once
#include <string>
#include <vector>
#include <optional>

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
        bool observer = false;
        bool prefer_sweatbox = false;
    };

    inline bool IsConnectionSessionForced(const ConnectionType connectionType) {
        return connectionType == CONNECTION_TYPE_PLAYBACK || connectionType == CONNECTION_TYPE_SWEATBOX;
    }

    inline std::string GetEffectiveSessionName(const ConnectionState& state) {
        if (state.connection_type == CONNECTION_TYPE_PLAYBACK) {
            return "PLAYBACK";
        }
        if (state.connection_type == CONNECTION_TYPE_SWEATBOX) {
            return "SWEATBOX";
        }
        return state.prefer_sweatbox ? "SWEATBOX" : "LIVE";
    }

    inline std::string GetEffectiveSessionShortName(const ConnectionState& state) {
        if (state.connection_type == CONNECTION_TYPE_PLAYBACK) {
            return "PBK";
        }
        return GetEffectiveSessionName(state) == "SWEATBOX" ? "SWX" : "LIVE";
    }

    struct VisibleController {
        std::string callsign;
        std::string primary_frequency;
        bool observer = false;
        bool operator==(const VisibleController&) const = default;
    };

    class IFlightStripsPlugin {
    public:
        virtual ~IFlightStripsPlugin() = default;
        virtual ConnectionState& GetConnectionState() = 0;
        virtual std::vector<std::string> GetEligibleAirports() const { return {}; }
        // Nullopt means the SDK connection/identity is not ready. An acquired
        // visible list includes non-plugin controllers and the local controller.
        virtual std::optional<std::vector<VisibleController>> GetVisibleControllers() const { return std::nullopt; }
        virtual void SetMembershipAirport(const std::string&,const std::string&,double,double) {}
        virtual void RemoveMembershipAirport(const std::string&) {}
    };
}
