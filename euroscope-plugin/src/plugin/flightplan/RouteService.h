#pragma once
#include "plugin/FlightStripsPluginInterface.h"

namespace FlightStrips::flightplan {
    class RouteService {
    public:
        explicit RouteService(const std::shared_ptr<FlightStripsPluginInterface> &m_plugin)
            : m_plugin(m_plugin) {
        }

        /// <summary>
        /// Modifies the route to set the specified SID.
        /// </summary>
        void SetSid(std::string& route, const std::string& sid, const std::string& airport) const;
    private:

        std::shared_ptr<FlightStripsPluginInterface> m_plugin;

        static std::string GetBaseSid(const std::string& sid);

    };

    // TODO move to util class
    static std::string &ltrim(std::string &s) {
        s.erase(s.begin(), std::ranges::find_if(s, [](const int c) {return !std::isspace(c);}));
        return s;
    }
}
